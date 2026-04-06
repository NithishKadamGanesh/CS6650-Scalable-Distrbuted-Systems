# ============================================================================
# build-lambda.ps1 - Compile Go Lambda and create zip for deployment
# Run from: C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw7\scripts
# ============================================================================

$PROJECT_DIR = $PSScriptRoot | Split-Path -Parent
$LAMBDA_DIR = "$PROJECT_DIR\order-lambda"

Write-Host "Building Lambda function..." -ForegroundColor Yellow

Set-Location $LAMBDA_DIR

# Generate go.sum
go mod tidy

# Cross-compile for Linux AMD64 (Lambda runs on Amazon Linux)
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

go build -o bootstrap .

# Reset environment
$env:GOOS = ""
$env:GOARCH = ""
$env:CGO_ENABLED = ""

if (-not (Test-Path "$LAMBDA_DIR\bootstrap")) {
    Write-Host "ERROR: Build failed - bootstrap binary not created" -ForegroundColor Red
    exit 1
}

Write-Host "  Binary built successfully" -ForegroundColor Green

# Create zip file for Lambda deployment
# Lambda with provided.al2 runtime expects a file named "bootstrap"
if (Test-Path "$LAMBDA_DIR\bootstrap.zip") {
    Remove-Item "$LAMBDA_DIR\bootstrap.zip"
}

Compress-Archive -Path "$LAMBDA_DIR\bootstrap" -DestinationPath "$LAMBDA_DIR\bootstrap.zip"

if (-not (Test-Path "$LAMBDA_DIR\bootstrap.zip")) {
    Write-Host "ERROR: Zip creation failed" -ForegroundColor Red
    exit 1
}

$zipSize = (Get-Item "$LAMBDA_DIR\bootstrap.zip").Length / 1MB
Write-Host "  bootstrap.zip created ($([math]::Round($zipSize, 2)) MB)" -ForegroundColor Green

# Clean up binary
Remove-Item "$LAMBDA_DIR\bootstrap"

Write-Host "Lambda build complete! Ready for terraform apply." -ForegroundColor Green
Set-Location $PROJECT_DIR
