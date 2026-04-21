#!/usr/bin/env bash
# restore_warehouse.sh — Restart a failed warehouse Redis container.

set -euo pipefail

WAREHOUSE=${1:-"b"}
CONTAINER="warehouseflow-redis-${WAREHOUSE}"

echo "Restoring WAREHOUSE ${WAREHOUSE^^}..."
docker start "$CONTAINER"

until docker exec "$CONTAINER" redis-cli ping 2>/dev/null | grep -q "PONG"; do
  sleep 1
done

echo "✓ WAREHOUSE ${WAREHOUSE^^} restored at $(date '+%H:%M:%S')"
echo "  Router will re-include it on next Ping() success."
