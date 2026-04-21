#!/usr/bin/env bash
# inject_failure.sh — Stop a warehouse Redis container to simulate failure.
# Usage: ./inject_failure.sh <a|b|c>

set -euo pipefail

WAREHOUSE=${1:-"b"}
CONTAINER="warehouseflow-redis-${WAREHOUSE}"

echo "================================================="
echo " WarehouseFlow — Failure Injection"
echo " Target : WAREHOUSE ${WAREHOUSE^^}"
echo " Time   : $(date '+%H:%M:%S')"
echo "================================================="

if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
  echo "ERROR: $CONTAINER not running."
  exit 1
fi

docker stop "$CONTAINER"
echo "✓ Warehouse ${WAREHOUSE^^} Redis stopped at $(date '+%H:%M:%S')"
echo "To restore: ./scripts/restore_warehouse.sh ${WAREHOUSE}"
