package router

// Engine tests — verify core routing logic using an in-memory Redis (miniredis)
// and a mock DecisionStore. No Docker or real Postgres required.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/nithishkadam/warehouseflow/routing-service/store"
)

// recordingStore satisfies store.DecisionStore and captures all decisions.
type recordingStore struct {
	decisions []store.RoutingDecision
}

func (r *recordingStore) InsertDecision(_ context.Context, d store.RoutingDecision) error {
	r.decisions = append(r.decisions, d)
	return nil
}

func (r *recordingStore) CountOversells(_ context.Context, sku string) (int, error) {
	count := 0
	for _, d := range r.decisions {
		if d.SKU == sku && d.Status == "routed" {
			count++
		}
	}
	return count, nil
}

// A US-WEST order should route to warehouse-c (region preference).
func TestRoutePrefersWarehouseForRegionAndPersistsDecision(t *testing.T) {
	registry, cleanup := newEngineTestRegistry(t)
	defer cleanup()

	if err := registry.SeedInventory(context.Background()); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	recorder := &recordingStore{}
	engine := NewEngine(registry, recorder)

	payload, _ := json.Marshal(OrderEvent{
		OrderID:    "order-1",
		CustomerID: "customer-1",
		SKU:        "SKU-ALPHA",
		Quantity:   1,
		Region:     "US-WEST",
	})

	if err := engine.Route(context.Background(), payload); err != nil {
		t.Fatalf("route order: %v", err)
	}

	if len(recorder.decisions) != 1 {
		t.Fatalf("expected 1 persisted decision, got %d", len(recorder.decisions))
	}
	d := recorder.decisions[0]
	if d.Status != "routed" {
		t.Fatalf("expected routed, got %s", d.Status)
	}
	if d.WarehouseID != "warehouse-c" {
		t.Fatalf("expected US-WEST → warehouse-c, got %s", d.WarehouseID)
	}
}

// Insufficient inventory should produce a rejected decision with a reason.
func TestRouteRejectsWhenInventoryUnavailable(t *testing.T) {
	registry, cleanup := newEngineTestRegistry(t)
	defer cleanup()

	// Seed only 1 unit of ALPHA, request 5 → should reject.
	if err := registry.SeedInventoryWithConfig(
		context.Background(),
		map[string]int64{"SKU-ALPHA": 1},
		2,
	); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	recorder := &recordingStore{}
	engine := NewEngine(registry, recorder)

	payload, _ := json.Marshal(OrderEvent{
		OrderID:    "order-2",
		CustomerID: "customer-2",
		SKU:        "SKU-ALPHA",
		Quantity:   5,
		Region:     "US-EAST",
	})

	if err := engine.Route(context.Background(), payload); err != nil {
		t.Fatalf("route order: %v", err)
	}

	if len(recorder.decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(recorder.decisions))
	}
	if recorder.decisions[0].Status != "rejected" {
		t.Fatalf("expected rejected, got %s", recorder.decisions[0].Status)
	}
	if recorder.decisions[0].FailReason == "" {
		t.Fatal("expected fail reason")
	}
}

// Helper: build a registry with 3 miniredis-backed warehouses.
func newEngineTestRegistry(t *testing.T) (*store.WarehouseRegistry, func()) {
	t.Helper()
	a := miniredis.RunT(t)
	b := miniredis.RunT(t)
	c := miniredis.RunT(t)

	registry := store.NewWarehouseRegistry([]store.WarehouseConfig{
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
