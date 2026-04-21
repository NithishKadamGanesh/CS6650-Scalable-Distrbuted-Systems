package store

// Interfaces used across the store package. Enables test doubles without
// pulling in real Postgres connections.

import "context"

// DecisionStore is the persistence interface the routing engine depends on.
type DecisionStore interface {
	InsertDecision(ctx context.Context, d RoutingDecision) error
	CountOversells(ctx context.Context, sku string) (int, error)
}
