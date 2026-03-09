#!/bin/bash
# =============================================================================
# Recover a crashed downstream service
#
#   soft: POST /admin/recover → tasks start passing health checks again
#         ALB re-registers them within ~15-30s
#
#   hard: Scale ECS service back to 2 → Fargate provisions new tasks
#         Full recovery: image pull + start + health check + Cloud Map = ~60-90s
#
# Usage:
#   ./scripts/recover-service.sh soft payment
#   ./scripts/recover-service.sh hard payment
# =============================================================================
set -euo pipefail

MODE="${1:-soft}"
SERVICE="${2:-payment}"
REGION="us-east-1"
CLUSTER="galactic-pizza-cluster"
ALB_DNS=$(terraform -chdir=terraform output -raw alb_dns_name)

case "$MODE" in
  soft)
    echo "🔧 Recovering ${SERVICE} via Order API..."
    curl -s -X POST "${ALB_DNS}/recover?service=${SERVICE}" | jq .
    echo ""
    echo "Tasks will pass health checks again."
    echo "ALB re-registers within ~15-30s."
    ;;

  hard)
    echo "🔧 Recovering ${SERVICE} — scaling back to 2 tasks..."
    aws ecs update-service \
      --cluster "$CLUSTER" \
      --service "galactic-pizza-${SERVICE}" \
      --desired-count 2 \
      --region "$REGION" > /dev/null
    echo "Done. Fargate is provisioning new tasks."
    echo "Recovery pipeline: pull image → start → health check → Cloud Map register"
    echo "Expect ~60-90s for full recovery."
    echo ""
    echo "Monitor:"
    echo "  watch -n5 'aws ecs describe-services --cluster $CLUSTER \\"
    echo "    --services galactic-pizza-${SERVICE} \\"
    echo "    --query \"services[].{running:runningCount,desired:desiredCount}\"'"
    ;;

  *)
    echo "Usage: $0 [soft|hard] [inventory|payment|kitchen]"
    exit 1
    ;;
esac
