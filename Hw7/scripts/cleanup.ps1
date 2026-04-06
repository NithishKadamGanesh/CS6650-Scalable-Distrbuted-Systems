# cleanup.ps1 - Tear down all AWS resources
$ErrorActionPreference = "Stop"
$PROJECT_DIR = $PSScriptRoot | Split-Path -Parent
$AWS_REGION = aws configure get region
if (-not $AWS_REGION) { $AWS_REGION = "us-east-1" }

Write-Host "  DESTROYING ALL AWS RESOURCES" -ForegroundColor Red
Write-Host ""
$confirm = Read-Host "Are you sure? Type 'yes' to continue"
if ($confirm -ne "yes") {
    Write-Host "Cancelled." -ForegroundColor Yellow
    exit 0
}

$ACCOUNT_ID = aws sts get-caller-identity --query Account --output text
$REGISTRY = "$ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
$RECEIVER_IMAGE = "$REGISTRY/async-orders/receiver:latest"
$PROCESSOR_IMAGE = "$REGISTRY/async-orders/processor:latest"

Write-Host ""
Write-Host "Destroying Terraform resources..." -ForegroundColor Yellow
Set-Location "$PROJECT_DIR\terraform"

terraform destroy -auto-approve `
    -var="receiver_image=$RECEIVER_IMAGE" `
    -var="processor_image=$PROCESSOR_IMAGE" `
    -var="aws_region=$AWS_REGION"

Write-Host ""
Write-Host "Deleting ECR repositories..." -ForegroundColor Yellow
aws ecr delete-repository --repository-name async-orders/receiver --force --region $AWS_REGION 2>$null
aws ecr delete-repository --repository-name async-orders/processor --force --region $AWS_REGION 2>$null

Write-Host ""
Write-Host "All resources destroyed!" -ForegroundColor Green
Set-Location $PROJECT_DIR
