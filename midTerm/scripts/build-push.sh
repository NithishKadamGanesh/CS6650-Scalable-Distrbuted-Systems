#!/bin/bash
# =============================================================================
# Build all Docker images and push to ECR, then force ECS redeployment
# Run from project root: ./scripts/build-push.sh
# =============================================================================
set -euo pipefail

REGION="us-east-1"
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR_BASE="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"
PROJECT="galactic-pizza"
CLUSTER="${PROJECT}-cluster"

echo "━━━ Authenticating with ECR ━━━"
aws ecr get-login-password --region "$REGION" | \
  docker login --username AWS --password-stdin "$ECR_BASE"

# ─── Build & Push Order API ───
echo ""
echo "━━━ Building Order API ━━━"
docker build -t "${PROJECT}/order-api" ./services/order-api/
docker tag "${PROJECT}/order-api:latest" "${ECR_BASE}/${PROJECT}/order-api:latest"
docker push "${ECR_BASE}/${PROJECT}/order-api:latest"
echo "✅ Order API pushed"

# ─── Build & Push Downstream Services ───
for SERVICE in inventory payment kitchen; do
  echo ""
  echo "━━━ Building ${SERVICE} ━━━"
  docker build -t "${PROJECT}/${SERVICE}" ./services/${SERVICE}/
  docker tag "${PROJECT}/${SERVICE}:latest" "${ECR_BASE}/${PROJECT}/${SERVICE}:latest"
  docker push "${ECR_BASE}/${PROJECT}/${SERVICE}:latest"
  echo "✅ ${SERVICE} pushed"
done

# ─── Force ECS to pull new images ───
echo ""
echo "━━━ Updating ECS services (force new deployment) ━━━"

aws ecs update-service --cluster "$CLUSTER" --service "${PROJECT}-order-api" \
  --force-new-deployment --region "$REGION" > /dev/null
echo "✅ Order API redeploying"

for SERVICE in inventory payment kitchen; do
  aws ecs update-service --cluster "$CLUSTER" --service "${PROJECT}-${SERVICE}" \
    --force-new-deployment --region "$REGION" > /dev/null
  echo "✅ ${SERVICE} redeploying"
done

echo ""
echo "━━━ Done! Monitor with: ━━━"
echo "  watch -n5 'aws ecs describe-services --cluster ${CLUSTER} \\"
echo "    --services ${PROJECT}-order-api ${PROJECT}-inventory ${PROJECT}-payment ${PROJECT}-kitchen \\"
echo "    --query \"services[].{name:serviceName,running:runningCount,desired:desiredCount}\" \\"
echo "    --output table'"
