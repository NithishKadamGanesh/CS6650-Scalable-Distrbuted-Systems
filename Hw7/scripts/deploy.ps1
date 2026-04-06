# deploy.ps1 - Full deployment script for Part II

$ErrorActionPreference = "Stop"
$PROJECT_DIR = $PSScriptRoot | Split-Path -Parent
Set-Location $PROJECT_DIR
Write-Host "  Part II: Async Order Processing Deployer" -ForegroundColor Cyan

# STEP 0: Verify prerequisites
Write-Host ""
Write-Host "[STEP 0] Verifying prerequisites..." -ForegroundColor Yellow

Write-Host "  Checking AWS credentials..."
$AWS_IDENTITY = aws sts get-caller-identity 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ERROR: AWS credentials not configured." -ForegroundColor Red
    exit 1
}
Write-Host "  AWS identity OK" -ForegroundColor Green

$ACCOUNT_ID = aws sts get-caller-identity --query Account --output text
$AWS_REGION = aws configure get region
if (-not $AWS_REGION) { $AWS_REGION = "us-east-1" }
$REGISTRY = "$ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"

Write-Host "  Account ID: $ACCOUNT_ID"
Write-Host "  Region: $AWS_REGION"
Write-Host "  ECR Registry: $REGISTRY"

Write-Host "  Checking Docker..."
docker info > $null 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ERROR: Docker is not running." -ForegroundColor Red
    exit 1
}
Write-Host "  Docker is running" -ForegroundColor Green

Write-Host "  Checking Go..."
go version
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ERROR: Go not found." -ForegroundColor Red
    exit 1
}

Write-Host "  Checking Terraform..."
terraform version
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ERROR: Terraform not found." -ForegroundColor Red
    exit 1
}

Write-Host "  All prerequisites OK!" -ForegroundColor Green


# STEP 1: Create ECR Repositories

Write-Host ""
Write-Host "[STEP 1] Creating ECR repositories..." -ForegroundColor Yellow

aws ecr create-repository --repository-name async-orders/receiver --region $AWS_REGION 2>$null
Write-Host "  async-orders/receiver - ready"

aws ecr create-repository --repository-name async-orders/processor --region $AWS_REGION 2>$null
Write-Host "  async-orders/processor - ready"


# STEP 2: Build Go modules (generate go.sum)

Write-Host ""
Write-Host "[STEP 2] Running go mod tidy..." -ForegroundColor Yellow

Set-Location "$PROJECT_DIR\order-receiver"
go mod tidy
Write-Host "  order-receiver go.sum generated" -ForegroundColor Green

Set-Location "$PROJECT_DIR\order-processor"
go mod tidy
Write-Host "  order-processor go.sum generated" -ForegroundColor Green

Set-Location $PROJECT_DIR


# STEP 3: Authenticate Docker to ECR

Write-Host ""
Write-Host "[STEP 3] Authenticating Docker to ECR..." -ForegroundColor Yellow

$ECR_PASSWORD = aws ecr get-login-password --region $AWS_REGION
$ECR_PASSWORD | docker login --username AWS --password-stdin $REGISTRY
Write-Host "  Docker authenticated to ECR" -ForegroundColor Green


# STEP 4: Build and push Docker images

Write-Host ""
Write-Host "[STEP 4] Building and pushing Docker images..." -ForegroundColor Yellow

$RECEIVER_IMAGE = "$REGISTRY/async-orders/receiver:latest"
$PROCESSOR_IMAGE = "$REGISTRY/async-orders/processor:latest"

Write-Host "  Building order-receiver..."
Set-Location "$PROJECT_DIR\order-receiver"
docker build -t $RECEIVER_IMAGE .
if ($LASTEXITCODE -ne 0) { Write-Host "  Build failed!" -ForegroundColor Red; exit 1 }

Write-Host "  Pushing order-receiver..."
docker push $RECEIVER_IMAGE
if ($LASTEXITCODE -ne 0) { Write-Host "  Push failed!" -ForegroundColor Red; exit 1 }
Write-Host "  order-receiver pushed" -ForegroundColor Green

Write-Host "  Building order-processor..."
Set-Location "$PROJECT_DIR\order-processor"
docker build -t $PROCESSOR_IMAGE .
if ($LASTEXITCODE -ne 0) { Write-Host "  Build failed!" -ForegroundColor Red; exit 1 }

Write-Host "  Pushing order-processor..."
docker push $PROCESSOR_IMAGE
if ($LASTEXITCODE -ne 0) { Write-Host "  Push failed!" -ForegroundColor Red; exit 1 }
Write-Host "  order-processor pushed" -ForegroundColor Green

Set-Location $PROJECT_DIR


# STEP 5: Terraform deploy (Phase 3 - 1 worker)

Write-Host ""
Write-Host "[STEP 5] Deploying infrastructure with Terraform..." -ForegroundColor Yellow

Set-Location "$PROJECT_DIR\terraform"

Write-Host "  terraform init..."
terraform init

Write-Host "  terraform apply (worker_count=1)..."
terraform apply -auto-approve `
  -var="receiver_image=$RECEIVER_IMAGE" `
  -var="processor_image=$PROCESSOR_IMAGE" `
  -var="worker_count=1" `
  -var="aws_region=$AWS_REGION"

if ($LASTEXITCODE -ne 0) {
    Write-Host "  Terraform apply failed!" -ForegroundColor Red
    exit 1
}

$ALB_URL = terraform output -raw alb_url
Write-Host ""

Write-Host "  ALB URL: $ALB_URL" -ForegroundColor Green


Set-Location $PROJECT_DIR


# STEP 6: Wait for ECS tasks to become healthy

Write-Host ""
Write-Host "[STEP 6] Waiting for ECS tasks to start (2-3 minutes)..." -ForegroundColor Yellow

$MAX_RETRIES = 30
$RETRY = 0
$HEALTHY = $false

while ((-not $HEALTHY) -and ($RETRY -lt $MAX_RETRIES)) {
    Start-Sleep -Seconds 10
    $RETRY++
    Write-Host "  Attempt $RETRY of $MAX_RETRIES - checking health endpoint..."

    try {
        $healthUrl = "$ALB_URL/health"
        $response = Invoke-RestMethod -Uri $healthUrl -TimeoutSec 5 -ErrorAction SilentlyContinue
        if ($response.status -eq "healthy") {
            $HEALTHY = $true
            Write-Host "  Service is healthy!" -ForegroundColor Green
        }
    }
    catch {
        Write-Host "  Not ready yet..." -ForegroundColor Gray
    }
}

if (-not $HEALTHY) {
    Write-Host "  WARNING: Service did not become healthy in time." -ForegroundColor Red
    Write-Host "  Check the ECS console. You may need to wait longer." -ForegroundColor Red
}


# STEP 7: Quick smoke test

Write-Host ""
Write-Host "[STEP 7] Running quick smoke test..." -ForegroundColor Yellow

$testBody = '{"customer_id":1234,"items":[{"product_id":"P1","name":"Widget","quantity":1,"price":9.99}]}'

Write-Host "  Testing sync endpoint (expect ~3s delay)..."
try {
    $syncUrl = "$ALB_URL/orders/sync"
    $syncResult = Invoke-RestMethod -Uri $syncUrl -Method POST -Body $testBody -ContentType "application/json" -TimeoutSec 30
    Write-Host "  SYNC OK - order completed" -ForegroundColor Green
}
catch {
    Write-Host "  SYNC FAILED - check ECS logs" -ForegroundColor Red
}

Write-Host "  Testing async endpoint (expect instant response)..."
try {
    $asyncUrl = "$ALB_URL/orders/async"
    $asyncResult = Invoke-RestMethod -Uri $asyncUrl -Method POST -Body $testBody -ContentType "application/json" -TimeoutSec 10
    Write-Host "  ASYNC OK - order accepted" -ForegroundColor Green
}
catch {
    Write-Host "  ASYNC FAILED - check ECS logs" -ForegroundColor Red
}


# DONE


Write-Host "  DEPLOYMENT COMPLETE!" -ForegroundColor Cyan
Write-Host ""
Write-Host "ALB URL: $ALB_URL" -ForegroundColor Green
Write-Host ""
Write-Host "Now run the load tests:" -ForegroundColor Yellow
Write-Host "  .\scripts\run-tests.ps1" -ForegroundColor White
Write-Host ""
Write-Host "When finished, clean up with:" -ForegroundColor Yellow
Write-Host "  .\scripts\cleanup.ps1" -ForegroundColor White

# Save ALB URL for test script
$ALB_URL | Out-File -FilePath "$PROJECT_DIR\scripts\alb_url.txt" -NoNewline
