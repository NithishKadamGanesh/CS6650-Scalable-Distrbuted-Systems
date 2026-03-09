output "alb_dns_name" {
  description = "Public ALB URL — point dashboard and Locust here"
  value       = "http://${aws_lb.main.dns_name}"
}

output "ecr_order_api" {
  description = "ECR repository URL for Order API"
  value       = aws_ecr_repository.order_api.repository_url
}

output "ecr_downstream" {
  description = "ECR repository URLs for downstream services"
  value       = { for k, v in aws_ecr_repository.downstream : k => v.repository_url }
}

output "ecs_cluster_name" {
  description = "ECS cluster name"
  value       = aws_ecs_cluster.main.name
}

output "vpc_id" {
  description = "VPC ID"
  value       = aws_vpc.main.id
}
