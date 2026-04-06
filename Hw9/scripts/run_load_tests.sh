#!/bin/bash
# =============================================================================
# run_load_tests.sh
# Runs load tests for all 4 database configurations at all 4 write ratios.
# Prerequisites: Both docker-compose clusters must be running.
# =============================================================================

set -e

LEADER_ADDR="localhost:8080"
FOLLOWER_ADDRS="localhost:8081,localhost:8082,localhost:8083,localhost:8084"
ALL_LF_ADDRS="localhost:8080,localhost:8081,localhost:8082,localhost:8083,localhost:8084"
LEADERLESS_ADDRS="localhost:9080,localhost:9081,localhost:9082,localhost:9083,localhost:9084"

NUM_REQUESTS=1000
CONCURRENCY=10
NUM_KEYS=10

RESULTS_DIR="results"
mkdir -p "$RESULTS_DIR"

# Helper: configure leader-follower W/R via API
configure_lf() {
    local w=$1 r=$2
    curl -s -X POST "http://$LEADER_ADDR/configure" \
        -H "Content-Type: application/json" \
        -d "{\"w\":$w,\"r\":$r}" > /dev/null
    echo "  Configured Leader-Follower: W=$w, R=$r"
}

# Helper: run load test
run_test() {
    local write_nodes=$1
    local read_nodes=$2
    local write_pct=$3
    local output=$4
    local label=$5

    echo "  Running: $label (write_pct=$write_pct%)"
    WRITE_NODES="$write_nodes" \
    READ_NODES="$read_nodes" \
    WRITE_PCT="$write_pct" \
    NUM_REQUESTS="$NUM_REQUESTS" \
    CONCURRENCY="$CONCURRENCY" \
    NUM_KEYS="$NUM_KEYS" \
    OUTPUT_FILE="$output" \
    go run loadtester/main.go
    echo "  -> Saved to $output"
    echo ""
}

echo "============================================="
echo "  DISTRIBUTED KV LOAD TESTING"
echo "============================================="
echo ""

WRITE_PCTS=(1 10 50 90)

# -----------------------------------------------
# Config 1: Leader-Follower W=5, R=1
# -----------------------------------------------
echo "=== Config: Leader-Follower W=5 R=1 ==="
configure_lf 5 1
sleep 1
for pct in "${WRITE_PCTS[@]}"; do
    run_test "$LEADER_ADDR" "$ALL_LF_ADDRS" "$pct" \
        "$RESULTS_DIR/results_w5r1_${pct}pct.csv" "W5R1 @ ${pct}% writes"
done

# -----------------------------------------------
# Config 2: Leader-Follower W=1, R=5
# -----------------------------------------------
echo "=== Config: Leader-Follower W=1 R=5 ==="
configure_lf 1 5
sleep 1
for pct in "${WRITE_PCTS[@]}"; do
    run_test "$LEADER_ADDR" "$ALL_LF_ADDRS" "$pct" \
        "$RESULTS_DIR/results_w1r5_${pct}pct.csv" "W1R5 @ ${pct}% writes"
done

# -----------------------------------------------
# Config 3: Leader-Follower W=3, R=3 (Quorum)
# -----------------------------------------------
echo "=== Config: Leader-Follower W=3 R=3 ==="
configure_lf 3 3
sleep 1
for pct in "${WRITE_PCTS[@]}"; do
    run_test "$LEADER_ADDR" "$ALL_LF_ADDRS" "$pct" \
        "$RESULTS_DIR/results_w3r3_${pct}pct.csv" "W3R3 @ ${pct}% writes"
done

# -----------------------------------------------
# Config 4: Leaderless W=N, R=1
# -----------------------------------------------
echo "=== Config: Leaderless W=N R=1 ==="
for pct in "${WRITE_PCTS[@]}"; do
    run_test "$LEADERLESS_ADDRS" "$LEADERLESS_ADDRS" "$pct" \
        "$RESULTS_DIR/results_leaderless_${pct}pct.csv" "Leaderless @ ${pct}% writes"
done

echo "============================================="
echo "  ALL TESTS COMPLETE"
echo "============================================="
echo "Results in $RESULTS_DIR/"
echo ""
echo "Generate graphs with:"
echo "  python3 scripts/generate_graphs.py $RESULTS_DIR/*.csv"
