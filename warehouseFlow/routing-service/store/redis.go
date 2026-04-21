package store

// Redis-backed warehouse state: inventory (atomic via Lua) and picker queue
// (LPOP/RPUSH — naturally serialized). Each warehouse has its own Redis
// instance to enable independent failure scenarios in Experiment 2.

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

// WarehouseConfig describes one warehouse's connection info.
type WarehouseConfig struct {
	ID        string
	Region    string
	RedisAddr string
}

// Warehouse wraps one warehouse's Redis client.
type Warehouse struct {
	ID     string
	Region string
	client *redis.Client
}

// WarehouseRegistry holds all warehouse nodes and provides region-aware ordering.
type WarehouseRegistry struct {
	warehouses []*Warehouse
	mu         sync.RWMutex
}

// NewWarehouseRegistry creates a registry with one client per warehouse.
func NewWarehouseRegistry(configs []WarehouseConfig) *WarehouseRegistry {
	reg := &WarehouseRegistry{}
	for _, cfg := range configs {
		client := redis.NewClient(&redis.Options{
			Addr: cfg.RedisAddr,
		})
		reg.warehouses = append(reg.warehouses, &Warehouse{
			ID:     cfg.ID,
			Region: cfg.Region,
			client: client,
		})
		log.Printf("Registered %s (%s) at %s", cfg.ID, cfg.Region, cfg.RedisAddr)
	}
	return reg
}

// GetAll returns all warehouses in registration order.
func (r *WarehouseRegistry) GetAll() []*Warehouse {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.warehouses
}

// GetPrioritized returns warehouses ordered so that the region-preferred
// warehouse comes first. Used for region-aware routing.
func (r *WarehouseRegistry) GetPrioritized(region string) []*Warehouse {
	r.mu.RLock()
	defer r.mu.RUnlock()

	preferredOrder := regionPreference(region)
	prioritized := make([]*Warehouse, 0, len(r.warehouses))

	for _, pref := range preferredOrder {
		for _, w := range r.warehouses {
			if w.ID == pref {
				prioritized = append(prioritized, w)
				break
			}
		}
	}

	// Append any warehouses not yet included (preserves them as fallbacks).
	for _, w := range r.warehouses {
		found := false
		for _, p := range prioritized {
			if p.ID == w.ID {
				found = true
				break
			}
		}
		if !found {
			prioritized = append(prioritized, w)
		}
	}

	return prioritized
}

// regionPreference returns warehouse IDs in preferred order for a region.
// Closer = earlier in the list. Falls through to alphabetical order for
// unknown regions.
func regionPreference(region string) []string {
	switch region {
	case "US-EAST":
		return []string{"warehouse-a", "warehouse-b", "warehouse-c"}
	case "US-CENTRAL":
		return []string{"warehouse-b", "warehouse-a", "warehouse-c"}
	case "US-WEST":
		return []string{"warehouse-c", "warehouse-b", "warehouse-a"}
	default:
		return []string{"warehouse-a", "warehouse-b", "warehouse-c"}
	}
}

// GetInventory returns available units for a SKU at this warehouse.
func (w *Warehouse) GetInventory(ctx context.Context, sku string) (int64, error) {
	key := fmt.Sprintf("inventory:%s", sku)
	val, err := w.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// DecrementInventory atomically checks-and-decrements inventory for a SKU.
// Returns remaining units. Error on insufficient inventory — callers never
// see negative inventory even under extreme concurrency.
func (w *Warehouse) DecrementInventory(ctx context.Context, sku string, quantity int) (int64, error) {
	key := fmt.Sprintf("inventory:%s", sku)
	script := redis.NewScript(`
		local current = tonumber(redis.call("GET", KEYS[1]))
		if current == nil then return redis.error_reply("NO_INVENTORY") end
		if current < tonumber(ARGV[1]) then return redis.error_reply("INSUFFICIENT_INVENTORY") end
		return redis.call("DECRBY", KEYS[1], ARGV[1])
	`)
	result, err := script.Run(ctx, w.client, []string{key}, quantity).Int64()
	return result, err
}

// GetAvailablePicker pops a picker ID from the warehouse's available queue.
func (w *Warehouse) GetAvailablePicker(ctx context.Context) (string, error) {
	pickerID, err := w.client.LPop(ctx, "pickers:available").Result()
	if err == redis.Nil {
		return "", fmt.Errorf("no pickers available at %s", w.ID)
	}
	return pickerID, err
}

// ReleasePicker returns a picker to the end of the available queue.
func (w *Warehouse) ReleasePicker(ctx context.Context, pickerID string) error {
	return w.client.RPush(ctx, "pickers:available", pickerID).Err()
}

// Ping checks warehouse reachability. Used for lazy health checks.
func (w *Warehouse) Ping(ctx context.Context) error {
	return w.client.Ping(ctx).Err()
}

// SeedInventory populates default SKUs + pickers if not already set (idempotent).
func (r *WarehouseRegistry) SeedInventory(ctx context.Context) error {
	return r.SeedInventoryWithConfig(ctx, map[string]int64{
		"SKU-ALPHA":   1000,
		"SKU-BETA":    800,
		"SKU-GAMMA":   600,
		"SKU-HOTITEM": 100,
	}, 20)
}

// SeedInventoryWithConfig allows custom SKU + picker counts for tests.
func (r *WarehouseRegistry) SeedInventoryWithConfig(ctx context.Context, skus map[string]int64, pickerCount int) error {
	for _, w := range r.warehouses {
		for sku, qty := range skus {
			key := fmt.Sprintf("inventory:%s", sku)
			w.client.SetNX(ctx, key, qty, 0)
		}

		pickerQueueLen, _ := w.client.LLen(ctx, "pickers:available").Result()
		if pickerQueueLen == 0 {
			for i := 1; i <= pickerCount; i++ {
				pickerID := fmt.Sprintf("%s-picker-%02d", w.ID, i)
				w.client.RPush(ctx, "pickers:available", pickerID)
			}
		}

		log.Printf("Seeded %s: %d SKUs + %d pickers", w.ID, len(skus), pickerCount)
	}
	return nil
}

// CloseAll closes all Redis connections.
func (r *WarehouseRegistry) CloseAll() {
	for _, w := range r.warehouses {
		w.client.Close()
	}
}
