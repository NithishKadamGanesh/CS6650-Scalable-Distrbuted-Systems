# Hummingbird API

A REST API to upload images, track metadata, and download a processed version.

### What it does

- **Upload** an image → stored in **S3** under `uploads/<mediaId>/<filename>`
- **Metadata** stored in **DynamoDB** (includes `status`)
- **Processing** creates a processed object in S3 under `resized/<mediaId>/<filename>` and updates status to `COMPLETE`
- **Download** returns a redirect (presigned S3 URL) to the processed object

### 1) AWS CLI login / credentials

Use either SSO or access keys.

**Option A: AWS SSO**

```bash
aws configure sso
aws sso login
aws sts get-caller-identity
```

**Option B: Access keys**

```bash
aws configure
aws sts get-caller-identity
```

### 2) Deploy infrastructure (Terraform)

From repo root:

```bash
cd terraform
terraform init
terraform apply --auto-approve
```

Get key outputs:

```bash
terraform output -raw alb_dns_name
terraform output -raw ecr_repository_url
terraform output -raw ecs_cluster_name
```

### 3) Build + push the Docker image to ECR

Login to ECR (update the account id/region if yours differs):

```bash
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin <your-accoutn-id>.dkr.ecr.us-west-2.amazonaws.com
```

(If needed) create/verify the ECR repo:

```bash
aws ecr describe-repositories --region us-west-2 --repository-names hummingbird-production-api
# if not found:
aws ecr create-repository --region us-west-2 --repository-name hummingbird-production-api
```

Build:

```bash
docker build -t hummingbird-api .
```

Tag:

```bash
docker tag hummingbird-api:latest <your-accoutn-id>.dkr.ecr.us-west-2.amazonaws.com/hummingbird-production-api:latest
```

Push:

```bash
docker push <your-accoutn-id>.dkr.ecr.us-west-2.amazonaws.com/hummingbird-production-api:latest
```

### 4) Verify it’s running

```bash
# health
curl http://<alb_dns_name>/health

# upload (example)
curl -X POST "http://<alb_dns_name>/v1/media/upload?width=500" -F "file=@C:\path\to\image.jpg"

# status
curl http://<alb_dns_name>/v1/media/<mediaId>/status

# download (when COMPLETE)
curl -i http://<alb_dns_name>/v1/media/<mediaId>/download
```

## Architecture (high level)

- **Express API**: `server.js`, `routes/`, `controllers/`
- **Storage**: `clients/s3.js`
- **Metadata**: `clients/dynamodb.js`
- **Events**: `clients/sns.js` publishes `media.v1.resize` and `media.v1.delete`
- **Worker (background processor)**: `worker/processor.js` (consumes events and marks media `COMPLETE`)

### Status + download semantics

- Upload creates DynamoDB record with status **`PENDING`**
- `GET /v1/media/:id/download`:
  - returns **202** until status is **`COMPLETE`**
  - returns **302** redirect to a presigned S3 URL once **`COMPLETE`**

### Terraform (ECS Fargate + ALB)

- Terraform lives in `terraform/` and deploys the API (and worker) to ECS Fargate behind an ALB.

## API Endpoints

- `POST /v1/media/upload` - Upload an image (multipart form-data `file`)
- `GET /v1/media/:id` - Get media metadata
- `GET /v1/media/:id/status` - Get processing status (`PENDING|PROCESSING|COMPLETE|ERROR`)
- `GET /v1/media/:id/download` - Download processed media (redirect)
- `PUT /v1/media/:id/resize` - Trigger processing (width optional)
- `DELETE /v1/media/:id` - Delete media
- `GET /health` - Health check

---

## 5) Set up Claude Code in AWS CloudShell

### Install Claude Code

```bash
sudo npm install -g @anthropic-ai/claude-code
```

### Configure AWS Bedrock

Claude Code can use Anthropic models hosted on AWS Bedrock — no separate API key needed,
it uses your existing AWS credentials.

Follow the Bedrock configuration guide:
https://code.claude.com/docs/en/amazon-bedrock

### Set environment variables

Run these in your CloudShell session before starting Claude Code:

```bash
export CLAUDE_CODE_USE_BEDROCK=1
export AWS_REGION=<your-aws-region>
export ANTHROPIC_MODEL='us.anthropic.claude-opus-4-6-v1'
```

This tells Claude Code to route requests through AWS Bedrock using your current AWS credentials.

### Start Claude Code

```bash
claude
```
