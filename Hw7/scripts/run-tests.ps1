# ============================================================================
# run-tests.ps1 - Run all Locust load tests phase by phase
# Run from: C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw7\scripts
# ============================================================================

$ErrorActionPreference = "Stop"
$PROJECT_DIR = $PSScriptRoot | Split-Path -Parent

# Read ALB URL from deploy output
$ALB_URL_FILE = "$PSScriptRoot\alb_url.txt"
if (Test-Path $ALB_URL_FILE) {
    $ALB_URL = (Get-Content $ALB_URL_FILE -Raw).Trim()
} else {
    $ALB_URL = Read-Host "Enter your ALB URL (e.g. http://async-orders-alb-123456.us-east-1.elb.amazonaws.com)"
}

Write-Host "Using ALB URL: $ALB_URL" -ForegroundColor Cyan

# Create results directory
$RESULTS_DIR = "$PROJECT_DIR\loadtest\results"
if (-not (Test-Path $RESULTS_DIR)) {
    New-Item -ItemType Directory -Path $RESULTS_DIR | Out-Null
}

Set-Location "$PROJECT_DIR\loadtest"

# ============================================================================
# PHASE 1a: Sync - Normal Operations (5 users, 30 seconds)
# ============================================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PHASE 1a: Sync - Normal (5 users, 30s)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Expected: 100% success, ~3s response times" -ForegroundColor Gray
Read-Host "Press Enter to start"

locust -f locustfile.py SyncUser `
    --host $ALB_URL `
    --users 5 --spawn-rate 1 --run-time 30s `
    --headless --csv results/sync_normal

Write-Host ""
Write-Host "Phase 1a complete! Check results/sync_normal_stats.csv" -ForegroundColor Green

# ============================================================================
# PHASE 1b: Sync - Flash Sale (20 users, 60 seconds)
# ============================================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PHASE 1b: Sync - Flash Sale (20 users, 60s)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Expected: Massive failures, timeouts, long response times" -ForegroundColor Gray
Write-Host ">>> SCREENSHOT #1: Screenshot the Locust summary when done <<<" -ForegroundColor Yellow
Read-Host "Press Enter to start"

locust -f locustfile.py SyncUser `
    --host $ALB_URL `
    --users 20 --spawn-rate 10 --run-time 60s `
    --headless --csv results/sync_flash

Write-Host ""
Write-Host "Phase 1b complete!" -ForegroundColor Green
Write-Host ">>> TAKE SCREENSHOT #1 NOW: Locust terminal showing failures <<<" -ForegroundColor Yellow
Read-Host "Press Enter after you have taken the screenshot"

# ============================================================================
# PHASE 3: Async - Flash Sale (20 users, 60 seconds, 1 worker)
# ============================================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PHASE 3: Async - Flash Sale (20 users, 60s, 1 worker)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Expected: 100% success, <100ms response times" -ForegroundColor Gray
Write-Host ">>> SCREENSHOT #2: Screenshot Locust summary when done <<<" -ForegroundColor Yellow
Read-Host "Press Enter to start"

locust -f locustfile.py AsyncUser `
    --host $ALB_URL `
    --users 20 --spawn-rate 10 --run-time 60s `
    --headless --csv results/async_flash_1w

Write-Host ""
Write-Host "Phase 3 complete!" -ForegroundColor Green
Write-Host ">>> TAKE SCREENSHOT #2 NOW: Locust terminal showing 100% success <<<" -ForegroundColor Yellow
Read-Host "Press Enter after you have taken the screenshot"

# ============================================================================
# PHASE 4: Check queue depth
# ============================================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PHASE 4: Queue Depth Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

Set-Location "$PROJECT_DIR\terraform"
$QUEUE_URL_RAW = terraform output -raw sqs_queue_url
Set-Location "$PROJECT_DIR\loadtest"

Write-Host "Queue URL: $QUEUE_URL_RAW"
aws sqs get-queue-attributes --queue-url $QUEUE_URL_RAW --attribute-names ApproximateNumberOfMessagesVisible ApproximateNumberOfMessagesNotVisible

Write-Host ""
Write-Host ">>> TAKE SCREENSHOT #3 NOW <<<" -ForegroundColor Yellow
Write-Host "Go to AWS Console -> CloudWatch -> Metrics -> SQS" -ForegroundColor Yellow
Write-Host "Select ApproximateNumberOfMessagesVisible for order-processing-queue" -ForegroundColor Yellow
Write-Host "You should see a massive spike from the 1-worker test" -ForegroundColor Yellow
Read-Host "Press Enter after you have taken the screenshot"

# ============================================================================
# Helper variables for terraform apply
# ============================================================================
$AWS_REGION = aws configure get region
if (-not $AWS_REGION) { $AWS_REGION = "us-east-1" }
$ACCOUNT_ID = aws sts get-caller-identity --query Account --output text
$REGISTRY = "$ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
$RECEIVER_IMAGE = "$REGISTRY/async-orders/receiver:latest"
$PROCESSOR_IMAGE = "$REGISTRY/async-orders/processor:latest"

# ============================================================================
# PHASE 5a: Scale to 5 workers, re-test
# ============================================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PHASE 5a: Async - 5 workers" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

Write-Host "  Redeploying with worker_count=5..."
Set-Location "$PROJECT_DIR\terraform"
terraform apply -auto-approve `
    -var="receiver_image=$RECEIVER_IMAGE" `
    -var="processor_image=$PROCESSOR_IMAGE" `
    -var="worker_count=5" `
    -var="aws_region=$AWS_REGION"

Write-Host "  Waiting 60s for ECS task to restart..." -ForegroundColor Gray
Start-Sleep -Seconds 60

Set-Location "$PROJECT_DIR\loadtest"
Write-Host "  Running load test..."
locust -f locustfile.py AsyncUser `
    --host $ALB_URL `
    --users 20 --spawn-rate 10 --run-time 60s `
    --headless --csv results/async_flash_5w

Write-Host ""
Write-Host "Phase 5a complete!" -ForegroundColor Green
Write-Host ">>> TAKE SCREENSHOT #4 NOW: CloudWatch queue depth (5 workers) <<<" -ForegroundColor Yellow
Read-Host "Press Enter after you have taken the screenshot"

# ============================================================================
# PHASE 5b: Scale to 20 workers, re-test
# ============================================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PHASE 5b: Async - 20 workers" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

Set-Location "$PROJECT_DIR\terraform"
terraform apply -auto-approve `
    -var="receiver_image=$RECEIVER_IMAGE" `
    -var="processor_image=$PROCESSOR_IMAGE" `
    -var="worker_count=20" `
    -var="aws_region=$AWS_REGION"

Write-Host "  Waiting 60s for ECS task to restart..." -ForegroundColor Gray
Start-Sleep -Seconds 60

Set-Location "$PROJECT_DIR\loadtest"
locust -f locustfile.py AsyncUser `
    --host $ALB_URL `
    --users 20 --spawn-rate 10 --run-time 60s `
    --headless --csv results/async_flash_20w

Write-Host ""
Write-Host "Phase 5b complete!" -ForegroundColor Green
Write-Host ">>> TAKE SCREENSHOT #5 NOW: CloudWatch queue depth (20 workers) <<<" -ForegroundColor Yellow
Read-Host "Press Enter after you have taken the screenshot"

# ============================================================================
# PHASE 5c: Scale to 100 workers, re-test
# ============================================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PHASE 5c: Async - 100 workers" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

Set-Location "$PROJECT_DIR\terraform"
terraform apply -auto-approve `
    -var="receiver_image=$RECEIVER_IMAGE" `
    -var="processor_image=$PROCESSOR_IMAGE" `
    -var="worker_count=100" `
    -var="aws_region=$AWS_REGION"

Write-Host "  Waiting 60s for ECS task to restart..." -ForegroundColor Gray
Start-Sleep -Seconds 60

Set-Location "$PROJECT_DIR\loadtest"
locust -f locustfile.py AsyncUser `
    --host $ALB_URL `
    --users 20 --spawn-rate 10 --run-time 60s `
    --headless --csv results/async_flash_100w

Write-Host ""
Write-Host "Phase 5c complete!" -ForegroundColor Green
Write-Host ">>> TAKE SCREENSHOT #6 NOW: CloudWatch queue depth (100 workers) <<<" -ForegroundColor Yellow
Read-Host "Press Enter to finish"

# ============================================================================
# SUMMARY
# ============================================================================
Write-Host ""
Write-Host "============================================" -ForegroundColor Cyan
Write-Host "  ALL TESTS COMPLETE!" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Results saved in: $RESULTS_DIR" -ForegroundColor Green
Write-Host ""
Write-Host "CSV files generated:" -ForegroundColor Yellow
Get-ChildItem "$RESULTS_DIR\*_stats.csv" | ForEach-Object { Write-Host "  $_" }
Write-Host ""
Write-Host "You should have 6 screenshots:" -ForegroundColor Yellow
Write-Host "  #1: Locust sync flash sale (failures)"
Write-Host "  #2: Locust async flash sale (100% success)"
Write-Host "  #3: CloudWatch queue spike (1 worker)"
Write-Host "  #4: CloudWatch queue depth (5 workers)"
Write-Host "  #5: CloudWatch queue depth (20 workers)"
Write-Host "  #6: CloudWatch queue depth (100 workers)"
Write-Host ""
Write-Host "To clean up:" -ForegroundColor Red
Write-Host "  .\scripts\cleanup.ps1"
