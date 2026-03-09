output "alb_dns_name" {
  description = "Public DNS name of the load balancer."
  value       = aws_lb.this.dns_name
}

output "ecr_repository_url" {
  description = "ECR repository URL to push images to."
  value       = aws_ecr_repository.api.repository_url
}

output "s3_bucket_name" {
  description = "S3 bucket used for media."
  value       = aws_s3_bucket.media.bucket
}

output "dynamodb_table_name" {
  description = "DynamoDB table used for media metadata."
  value       = aws_dynamodb_table.media.name
}

output "sns_topic_arn" {
  description = "SNS topic ARN used for media events."
  value       = aws_sns_topic.media_management.arn
}

output "ecs_cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.this.name
}

output "media_events_queue_url" {
  description = "SQS queue URL that receives media events (SNS subscription)."
  value       = aws_sqs_queue.media_events.id
}
