# Final Mastery - ChaosArena Album Store

This folder contains a Go implementation of the `v1-album-store` assignment contract.

## Included

- Go HTTP service for all required endpoints
- SQLite persistence with WAL mode
- Async photo processing worker pool
- Public media endpoint for completed photo URLs
- Delete handling that removes both metadata and stored files
- Dockerfile and local run instructions
- Go tests for correctness-critical behavior
- AWS Terraform deployment under `terraform/`

## Run locally

```bash
cd final-mastery
go mod tidy
go run .
```

## Environment variables

- `ADDR` default `:8000`
- `PUBLIC_BASE_URL` default `http://localhost:8000`
- `DATA_DIR` default `./data`
- `PROCESSING_DELAY_MS` default `0`
- `MAX_WORKERS` default `4`

## Tests

```bash
cd final-mastery
go test ./...
```

## Docker

```bash
cd final-mastery
docker build -t final-mastery .
docker run -p 8000:8000 -e PUBLIC_BASE_URL=http://localhost:8000 final-mastery
```

## Deployment note

ChaosArena is in `us-west-2`. If you deploy your service and storage in a different region such as `us-east-1`, photo URL fetches can pick up extra cross-region latency and hurt the load-test score. For best results, deploy the service and any object/file storage as close to `us-west-2` as possible.

## Submission example

```bash
curl -X POST http://chaosarena-alb-938452724.us-west-2.elb.amazonaws.com/submit \
  -H "Content-Type: application/json" \
  -d '{
    "email": "kadamganesh.n@northeastern.edu",
    "nickname": "your-nickname",
    "base_url": "http://your-deployed-service-url",
    "contract": "v1-album-store"
  }'
```

```bash
curl http://chaosarena-alb-938452724.us-west-2.elb.amazonaws.com/runs/<run_id>
```
