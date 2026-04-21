#!/usr/bin/env bash
# run_experiment3.sh — Write contention under scarcity.

set -euo pipefail
REDIS_HOST="${REDIS_HOST:-localhost}"
ALB_HOST="${ALB_HOST:-localhost:8081}"
RESULTS_DIR="results/experiment3"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

echo "=== Experiment 3: Write Contention ==="

seed() {
  for PORT in 6379 6380 6381; do
    redis-cli -h "$REDIS_HOST" -p "$PORT" SET "inventory:SKU-HOTITEM" 100 > /dev/null
  done
}

count_routed() {
  PGPASSWORD=warehouseflow psql -h localhost -U warehouseflow -d warehouseflow \
    -t -c "SELECT COUNT(*) FROM routing_decisions WHERE sku='SKU-HOTITEM' AND status='routed';" \
    2>/dev/null | tr -d ' \n'
}

echo "strategy,run,routed,oversells" > "$RESULTS_DIR/exp3_oversells_${TIMESTAMP}.csv"

for STRATEGY in optimistic pessimistic; do
  for RUN in 1 2 3 4 5; do
    echo ""
    echo "--- $STRATEGY run $RUN ---"
    seed
    PGPASSWORD=warehouseflow psql -h localhost -U warehouseflow -d warehouseflow \
      -c "DELETE FROM routing_decisions WHERE sku='SKU-HOTITEM';" 2>/dev/null

    export CONCURRENCY_STRATEGY="$STRATEGY"
    OUT="$RESULTS_DIR/exp3_${STRATEGY}_run${RUN}_${TIMESTAMP}"
    locust -f load-tests/locustfile_exp3.py --headless \
      --host "http://$ALB_HOST" --users 1000 --spawn-rate 500 \
      --run-time 30s --csv "$OUT" 2>&1 | tail -5

    sleep 3
    ROUTED=$(count_routed)
    OVERSELLS=$((ROUTED - 100 > 0 ? ROUTED - 100 : 0))
    echo "  Routed: $ROUTED (expected 100, oversells: $OVERSELLS)"
    echo "$STRATEGY,$RUN,$ROUTED,$OVERSELLS" >> "$RESULTS_DIR/exp3_oversells_${TIMESTAMP}.csv"
    sleep 5
  done
done

echo ""
echo "=== Done ==="
cat "$RESULTS_DIR/exp3_oversells_${TIMESTAMP}.csv"
