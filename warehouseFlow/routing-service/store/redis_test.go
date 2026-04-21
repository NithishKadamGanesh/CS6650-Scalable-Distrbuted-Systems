package store

// Tests for region-aware warehouse prioritization and atomic decrement
// behavior using miniredis.

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// US-WEST orders should prefer warehouse-c first.
func TestGetPrioritizedUsesRegionOrder(t *testing.T) {
	registry, cleanup := newTestWarehouseRegistry(t)
	defer cleanup()

	ordered := registry.GetPrioritized("US-WEST")
	if len(ordered) != 3 {
		t.Fatalf("expected 3 warehouses, got %d", len(ordered))
	}
	expected := []string{"warehouse-c", "warehouse-b", "warehouse-a"}
	for i, w := range ordered {
		if w.ID != expected[i] {
			t.Fatalf("position %d: expected %s, got %s", i, expected[i], w.ID)
		}
	}
}

// Successful decrement should return the remaining count.
func TestDecrementInventoryReturnsRemainingUnits(t *testing.T) {
	registry, cleanup := newTestWarehouseRegistry(t)
	defer cleanup()

	if err := registry.SeedInventory(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := registry.GetAll()[0]
	remaining, err := w.DecrementInventory(context.Background(), "SKU-ALPHA", 5)
	if err != nil {
		t.Fatalf("decrement: %v", err)
	}
	if remaining != 995 {
		t.Fatalf("expected 995, got %d", remaining)
	}
}

// Decrement must reject when requesting more than available — core correctness guarantee.
func TestDecrementInventoryRejectsInsufficient(t *testing.T) {
	registry, cleanup := newTestWarehouseRegistry(t)
	defer cleanup()

	registry.SeedInventoryWithConfig(context.Background(), map[string]int64{"SKU-X": 3}, 1)

	w := registry.GetAll()[0]
	_, err := w.DecrementInventory(context.Background(), "SKU-X", 10)
	if err == nil {
		t.Fatal("expected error for insufficient inventory")
	}

	// Verify inventory was NOT touched
	remaining, _ := w.GetInventory(context.Background(), "SKU-X")
	if remaining != 3 {
		t.Fatalf("inventory should still be 3, got %d", remaining)
	}
}

// Lock acquisition + release cycle.
func TestAcquireLockBasic(t *testing.T) {
	registry, cleanup := newTestWarehouseRegistry(t)
	defer cleanup()

	w := registry.GetAll()[0]
	ok, err := w.AcquireLock(context.Background(), "test-lock", 0)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !ok {
		t.Fatal("first acquire should succeed")
	}

	// Second acquire should fail
	ok, _ = w.AcquireLock(context.Background(), "test-lock", 0)
	if ok {
		t.Fatal("second acquire should fail while locked")
	}

	// Release and reacquire
	w.ReleaseLock(context.Background(), "test-lock")
	ok, _ = w.AcquireLock(context.Background(), "test-lock", 0)
	if !ok {
		t.Fatal("reacquire after release should succeed")
	}
}

func newTestWarehouseRegistry(t *testing.T) (*WarehouseRegistry, func()) {
	t.Helper()
	a := miniredis.RunT(t)
	b := miniredis.RunT(t)
	c := miniredis.RunT(t)

	registry := NewWarehouseRegistry([]WarehouseConfig{
		{ID: "warehouse-a", Region: "US-EAST", RedisAddr: a.Addr()},
		{ID: "warehouse-b", Region: "US-CENTRAL", RedisAddr: b.Addr()},
		{ID: "warehouse-c", Region: "US-WEST", RedisAddr: c.Addr()},
	})

	cleanup := func() {
		registry.CloseAll()
		a.Close(); b.Close(); c.Close()
	}

	return registry, cleanup
}
