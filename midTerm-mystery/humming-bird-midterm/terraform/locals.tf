locals {
  # S3 bucket names must be globally unique. Account IDs are globally unique, so
  # this default should avoid collisions in most cases.
  media_bucket_name = var.bucket_name != "" ? var.bucket_name : "${var.project_name}-${var.environment}-${data.aws_caller_identity.current.account_id}"

  # Default image points at the module's ECR repo :latest.
  # Learner-lab friendly: you can `terraform apply` first, then push the image.
  image_uri_effective = var.image_uri != "" ? var.image_uri : "${aws_ecr_repository.api.repository_url}:latest"
}

