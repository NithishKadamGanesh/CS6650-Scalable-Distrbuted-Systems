#!/usr/bin/env bash
# init_db.sh — Create the routing_decisions audit table.
# Idempotent: safe to run multiple times.

set -euo pipefail

PG_HOST="${PG_HOST:-localhost}"
PG_PORT="${PG_PORT:-5432}"

echo "Initializing Postgres schema..."
PGPASSWORD=warehouseflow psql -h "$PG_HOST" -p "$PG_PORT" \
  -U warehouseflow -d warehouseflow <<EOF
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
EOF

echo "✓ Schema initialized."
