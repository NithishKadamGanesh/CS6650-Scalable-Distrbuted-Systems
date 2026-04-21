#!/usr/bin/env bash
# run_experiment1.sh — Throughput vs. ECS task count.

set -euo pipefail
ALB_HOST="${ALB_HOST:-localhost:8081}"
RESULTS_DIR="results/experiment1"
mkdir -p "$RESULTS_DIR"

echo "=== Experiment 1: Horizontal Scaling ==="

for TASKS in 1 2 4 8; do
  echo ""
  echo "--- $TASKS ECS task(s) ---"

  if [ "$ALB_HOST" != "localhost:8081" ]; then
    aws ecs update-service \
      --cluster warehouseflow-cluster \
      --service warehouseflow-routing \
      --desired-count "$TASKS" --no-cli-pager
    sleep 30
  fi

  OUT="$RESULTS_DIR/exp1_tasks${TASKS}_$(date +%s)"
  locust -f load-tests/locustfile_exp1.py --headless \
    --host "http://$ALB_HOST" --users 84 --spawn-rate 20 \
    --run-time 30s --csv "$OUT" --csv-full-history 2>&1 | tee "${OUT}.log"

  sleep 10
done

echo "=== Done. Next: python scripts/generate_charts.py ==="
