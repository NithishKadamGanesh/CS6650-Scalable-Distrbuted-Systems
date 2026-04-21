# Terraform Deployment

This Terraform setup deploys the Go album store to AWS ECS Fargate behind an ALB in `us-west-2`.

## What it creates

- VPC with 2 public subnets
- Internet gateway and public routing
- ALB with `/health` health check
- ECS cluster, task definition, and single Fargate service
- ECR repository for the container image
- CloudWatch log group

It reuses the existing Learner Lab IAM role named `LabRole` for ECS task execution so it does not need `iam:CreateRole` permissions.

## Important note

This service currently stores photos and SQLite data on local container disk. That means the deployment is intentionally configured as a single ECS task. If the task restarts, local state is lost. That is acceptable for a basic deployment, but not ideal for long-lived production data.

## Deploy steps

1. Initialize Terraform:

```bash
cd terraform
terraform init
```

2. Review variables:

```bash
copy terraform.tfvars.example terraform.tfvars
```

3. Create infrastructure:

```bash
terraform apply
```

4. Build and push the image to ECR:

```bash
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin <ecr_repository_url>
docker build -t final-mastery ..
docker tag final-mastery:latest <ecr_repository_url>:latest
docker push <ecr_repository_url>:latest
```

5. Force ECS to pull the new image:

```bash
aws ecs update-service --region us-west-2 --cluster <cluster_name> --service <service_name> --force-new-deployment
```

6. Use the `base_url` Terraform output for `PUBLIC_BASE_URL` checks and ChaosArena submission.
