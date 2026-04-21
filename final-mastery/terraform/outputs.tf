output "aws_region" {
  value = var.aws_region
}

output "ecr_repository_url" {
  value = aws_ecr_repository.app.repository_url
}

output "cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "service_name" {
  value = aws_ecs_service.app.name
}

output "alb_dns_name" {
  value = aws_lb.app.dns_name
}

output "base_url" {
  value = "http://${aws_lb.app.dns_name}"
}

output "s3_bucket" {
  value = aws_s3_bucket.photos.bucket
}

output "albums_table" {
  value = aws_dynamodb_table.albums.name
}

output "photos_table" {
  value = aws_dynamodb_table.photos.name
}
