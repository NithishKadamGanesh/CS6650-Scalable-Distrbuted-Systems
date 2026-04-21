#!/usr/bin/env bash
# run_experiment5.sh — Kafka partition ↔ consumer matrix.

set -euo pipefail
ALB_HOST="${ALB_HOST:-localhost:8081}"
RESULTS_DIR="results/experiment5"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

echo "=== Experiment 5: Kafka Parallelism ==="

create_topic() {
  local P=$1
  docker exec warehouseflow-redpanda rpk topic delete "orders_p${P}" 2>/dev/null || true
  docker exec warehouseflow-redpanda rpk topic create "orders_p${P}" \
    --partitions "$P" --replicas 1
}

for P in 1 3 6 12; do
  create_topic "$P"
  for C in 1 2 4 8; do
    [ "$C" -gt 12 ] && continue
    echo ""
    echo "--- Partitions=$P Consumers=$C ---"

    PIDS=()
    for i in $(seq 1 "$C"); do
      KAFKA_TOPIC="orders_p${P}" KAFKA_GROUP_ID="exp5-grp" PORT=$((9100 + i)) \
        go run routing-service/main.go 2>&1 >> "$RESULTS_DIR/exp5_p${P}_c${C}_consumer${i}.log" &
      PIDS+=($!)
    done
    sleep 8

    OUT="$RESULTS_DIR/exp5_p${P}_c${C}_${TIMESTAMP}"
    locust -f load-tests/locustfile_exp5.py --headless \
      --host "http://$ALB_HOST" --users 84 --spawn-rate 20 \
      --run-time 45s --csv "$OUT" 2>&1 | tail -5

    for PID in "${PIDS[@]}"; do kill "$PID" 2>/dev/null || true; done
    wait 2>/dev/null || true
    sleep 5
  done
done
echo "=== Done ==="
