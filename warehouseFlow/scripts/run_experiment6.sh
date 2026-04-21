#!/usr/bin/env bash
# run_experiment6.sh — Noisy neighbor latency test.

set -euo pipefail
ALB_HOST="${ALB_HOST:-localhost:8081}"
RESULTS_DIR="results/experiment6"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

LOG="$RESULTS_DIR/exp6_run_${TIMESTAMP}.log"
OUT="$RESULTS_DIR/exp6_locust_${TIMESTAMP}"

echo "=== Experiment 6: Noisy Neighbor ===" | tee "$LOG"
echo "60s baseline → 60s noisy burst → 60s recovery" | tee -a "$LOG"

locust -f load-tests/locustfile_exp6.py --headless \
  --host "http://$ALB_HOST" --users 100 --spawn-rate 25 \
  --run-time 180s --csv "$OUT" --csv-full-history 2>&1 | tee -a "$LOG"

echo "=== Done ===" | tee -a "$LOG"
