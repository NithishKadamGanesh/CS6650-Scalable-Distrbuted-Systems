#!/usr/bin/env bash
# run_experiment7.sh — Cold start after auto-scale.

set -euo pipefail
ALB_HOST="${ALB_HOST:-localhost:8081}"
CLUSTER="${CLUSTER:-warehouseflow-cluster}"
SERVICE="${SERVICE:-warehouseflow-routing}"
RESULTS_DIR="results/experiment7"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

LOG="$RESULTS_DIR/exp7_run_${TIMESTAMP}.log"
OUT="$RESULTS_DIR/exp7_locust_${TIMESTAMP}"

echo "=== Experiment 7: Cold Start ===" | tee "$LOG"

# Scale to 2 baseline
aws ecs update-service --cluster "$CLUSTER" --service "$SERVICE" \
  --desired-count 2 --no-cli-pager 2>&1 | tee -a "$LOG" || true
sleep 60

locust -f load-tests/locustfile_exp7.py --headless \
  --host "http://$ALB_HOST" --users 84 --spawn-rate 20 \
  --run-time 240s --csv "$OUT" --csv-full-history 2>&1 >> "$LOG" &
LOCUST_PID=$!

echo "Baseline 2-task for 60s..." | tee -a "$LOG"
sleep 60

echo "Scaling 2 → 4 at $(date '+%H:%M:%S')" | tee -a "$LOG"
aws ecs update-service --cluster "$CLUSTER" --service "$SERVICE" \
  --desired-count 4 --no-cli-pager 2>&1 | tee -a "$LOG"

echo "Warming 120s..." | tee -a "$LOG"
sleep 120

echo "Final 60s..." | tee -a "$LOG"
sleep 60

wait "$LOCUST_PID" 2>/dev/null || true
echo "=== Done ===" | tee -a "$LOG"
