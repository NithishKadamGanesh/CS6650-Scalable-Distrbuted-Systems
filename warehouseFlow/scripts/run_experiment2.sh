#!/usr/bin/env bash
# run_experiment2.sh — Resilience under node failure.
# Timeline: 30s baseline → failure → 90s degraded → restore → 60s recovery.

set -euo pipefail
ALB_HOST="${ALB_HOST:-localhost:8081}"
RESULTS_DIR="results/experiment2"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

LOG="$RESULTS_DIR/exp2_run_${TIMESTAMP}.log"
OUT="$RESULTS_DIR/exp2_locust_${TIMESTAMP}"

echo "=== Experiment 2: Resilience ===" | tee "$LOG"

locust -f load-tests/locustfile_exp2.py --headless \
  --host "http://$ALB_HOST" --users 84 --spawn-rate 20 \
  --run-time 180s --csv "$OUT" --csv-full-history 2>&1 >> "$LOG" &
LOCUST_PID=$!

echo "Baseline 30s..." | tee -a "$LOG"
sleep 30

echo "Injecting failure at $(date '+%H:%M:%S')" | tee -a "$LOG"
./scripts/inject_failure.sh b 2>&1 | tee -a "$LOG"
FAIL_T=$(date +%s)

echo "Degraded operation 90s..." | tee -a "$LOG"
sleep 90

echo "Restoring at $(date '+%H:%M:%S')" | tee -a "$LOG"
./scripts/restore_warehouse.sh b 2>&1 | tee -a "$LOG"
REC_T=$(date +%s)
echo "Degraded duration: $((REC_T - FAIL_T))s" | tee -a "$LOG"

echo "Observing recovery 60s..." | tee -a "$LOG"
sleep 60

wait "$LOCUST_PID" 2>/dev/null || true
echo "=== Done. Results: $OUT_stats.csv ===" | tee -a "$LOG"
