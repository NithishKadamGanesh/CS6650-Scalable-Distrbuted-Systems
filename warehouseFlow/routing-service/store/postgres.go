package store

// Postgres-backed routing audit log. Every routing decision —
// success or rejection — is persisted for correctness verification
// and post-hoc analysis.

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

// RoutingDecision is one row of the audit log.
type RoutingDecision struct {
	OrderID     string
	CustomerID  string
	SKU         string
	Quantity    int
	WarehouseID string
	PickerID    string
	RoutedAt    time.Time
	LatencyMs   int64
	Status      string
	FailReason  string
}

// Postgres wraps a sql.DB and provides audit-log operations.
type Postgres struct {
	db *sql.DB
}

// NewPostgres connects to the given DSN.
func NewPostgres(dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return &Postgres{db: db}, nil
}

// Migrate creates the audit table and indexes if they don't exist.
func (p *Postgres) Migrate() error {
	_, err := p.db.Exec(`
		CREATE TABLE IF NOT EXISTS routing_decisions (
			id           SERIAL PRIMARY KEY,
			order_id     VARCHAR(36) NOT NULL UNIQUE,
			customer_id  VARCHAR(255) NOT NULL,
			sku          VARCHAR(255) NOT NULL,
			quantity     INTEGER NOT NULL,
			warehouse_id VARCHAR(255),
			picker_id    VARCHAR(255),
			routed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			latency_ms   BIGINT NOT NULL DEFAULT 0,
			status       VARCHAR(50) NOT NULL DEFAULT 'routed',
			fail_reason  TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_routing_decisions_sku ON routing_decisions(sku);
		CREATE INDEX IF NOT EXISTS idx_routing_decisions_routed_at ON routing_decisions(routed_at);
		CREATE INDEX IF NOT EXISTS idx_routing_decisions_status ON routing_decisions(status);
		CREATE INDEX IF NOT EXISTS idx_routing_decisions_warehouse ON routing_decisions(warehouse_id);
	`)
	return err
}

// InsertDecision persists a routing decision. Idempotent on order_id.
func (p *Postgres) InsertDecision(ctx context.Context, d RoutingDecision) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO routing_decisions
			(order_id, customer_id, sku, quantity, warehouse_id, picker_id, routed_at, latency_ms, status, fail_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (order_id) DO NOTHING
	`,
		d.OrderID, d.CustomerID, d.SKU, d.Quantity,
		d.WarehouseID, d.PickerID, d.RoutedAt,
		d.LatencyMs, d.Status, d.FailReason,
	)
	return err
}

// CountOversells returns the number of successfully routed orders for a SKU.
// Used in Experiment 3 to verify correctness post-hoc.
func (p *Postgres) CountOversells(ctx context.Context, sku string) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM routing_decisions
		WHERE sku = $1 AND status = 'routed'
	`, sku).Scan(&count)
	return count, err
}

// Close closes the underlying DB connection pool.
func (p *Postgres) Close() {
	p.db.Close()
}
