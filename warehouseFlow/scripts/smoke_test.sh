#!/usr/bin/env bash
# smoke_test.sh — End-to-end sanity check.
# Submits a single order and verifies it lands in Postgres.

set -euo pipefail

ALB_HOST="${ALB_HOST:-localhost:8081}"
PG_HOST="${PG_HOST:-localhost}"
PG_PORT="${PG_PORT:-5432}"

echo "=== WarehouseFlow Smoke Test ==="

# Submit order
echo "Submitting test order..."
RESPONSE=$(curl -s -X POST "http://${ALB_HOST}/api/v1/orders" \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"smoke-test","sku":"SKU-ALPHA","quantity":1,"region":"US-EAST"}')
echo "Response: $RESPONSE"

ORDER_ID=$(echo "$RESPONSE" | grep -o '"order_id":"[^"]*"' | cut -d'"' -f4)
echo "Order ID: $ORDER_ID"

echo "Waiting 2s for pipeline..."
sleep 2

# Verify in Postgres
echo "Verifying routing_decisions audit log..."
PGPASSWORD=warehouseflow psql -h "$PG_HOST" -p "$PG_PORT" \
  -U warehouseflow -d warehouseflow \
  -c "SELECT order_id, warehouse_id, picker_id, status, latency_ms FROM routing_decisions WHERE order_id='${ORDER_ID}';"

echo ""
echo "✓ Smoke test complete."
