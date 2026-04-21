#!/usr/bin/env bash
# run_experiment4.sh — Network partition CAP test.

set -euo pipefail
ALB_HOST="${ALB_HOST:-localhost:8081}"
RESULTS_DIR="results/experiment4"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

LOG="$RESULTS_DIR/exp4_run_${TIMESTAMP}.log"
OUT="$RESULTS_DIR/exp4_locust_${TIMESTAMP}"

echo "=== Experiment 4: Network Partition ===" | tee "$LOG"

locust -f load-tests/locustfile_exp4.py --headless \
  --host "http://$ALB_HOST" --users 84 --spawn-rate 20 \
  --run-time 240s --csv "$OUT" --csv-full-history 2>&1 >> "$LOG" &
LOCUST_PID=$!

echo "Baseline 30s..." | tee -a "$LOG"
sleep 30

echo "Injecting partition at $(date '+%H:%M:%S')" | tee -a "$LOG"
docker network disconnect warehouseflow_default warehouseflow-redis-b 2>&1 | tee -a "$LOG" || true

echo "Partitioned for 120s..." | tee -a "$LOG"
sleep 120

echo "Healing at $(date '+%H:%M:%S')" | tee -a "$LOG"
docker network connect warehouseflow_default warehouseflow-redis-b 2>&1 | tee -a "$LOG" || true

echo "Reconciliation observation 60s..." | tee -a "$LOG"
sleep 60

wait "$LOCUST_PID" 2>/dev/null || true
echo "=== Done ===" | tee -a "$LOG"
