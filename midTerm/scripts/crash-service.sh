#!/bin/bash
# =============================================================================
# Crash a downstream service — two modes
#
#   soft: POST /admin/crash → tasks stay running but return 503
#         ALB health checks eventually drain them (~30-45s)
#
#   hard: Scale ECS service to 0 → no tasks exist at all
#         Cloud Map deregisters DNS entries. Total outage for that service.
#
# Usage:
#   ./scripts/crash-service.sh soft payment
#   ./scripts/crash-service.sh hard payment
# =============================================================================
set -euo pipefail

MODE="${1:-soft}"
SERVICE="${2:-payment}"
REGION="us-east-1"
CLUSTER="galactic-pizza-cluster"
ALB_DNS=$(terraform -chdir=terraform output -raw alb_dns_name)

case "$MODE" in
  soft)
    echo "💥 Soft-crashing ${SERVICE} via Order API..."
    curl -s -X POST "${ALB_DNS}/crash?service=${SERVICE}" | jq .
    echo ""
    echo "Tasks are still running but returning 503."
    echo "ALB will drain them after health checks fail (~30-45s)."
    echo "Circuit breaker in Order API will trip after 5 failures."
    ;;

  hard)
    echo "💥 Hard-crashing ${SERVICE} — scaling to 0 tasks..."
    aws ecs update-service \
      --cluster "$CLUSTER" \
      --service "galactic-pizza-${SERVICE}" \
      --desired-count 0 \
      --region "$REGION" > /dev/null
    echo "Done. ${SERVICE} has 0 running tasks."
    echo ""
    echo "Monitor:"
    echo "  aws ecs describe-services --cluster $CLUSTER \\"
    echo "    --services galactic-pizza-${SERVICE} \\"
    echo "    --query 'services[].{running:runningCount,desired:desiredCount}'"
    ;;

  *)
    echo "Usage: $0 [soft|hard] [inventory|payment|kitchen]"
    exit 1
    ;;
esac
