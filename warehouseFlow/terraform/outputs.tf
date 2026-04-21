output "alb_dns" {
  value       = aws_lb.main.dns_name
  description = "ALB endpoint — use as ALB_HOST when running experiments"
}

output "msk_bootstrap_brokers" {
  value = aws_msk_cluster.main.bootstrap_brokers
}

output "rds_endpoint" {
  value     = aws_db_instance.main.endpoint
  sensitive = true
}

output "elasticache_endpoints" {
  value = {
    a = aws_elasticache_cluster.warehouse["a"].cache_nodes[0].address
    b = aws_elasticache_cluster.warehouse["b"].cache_nodes[0].address
    c = aws_elasticache_cluster.warehouse["c"].cache_nodes[0].address
  }
}

output "sqs_dlq_url" {
  value = aws_sqs_queue.dlq.url
}

output "ecr_repos" {
  value = {
    ingestion = aws_ecr_repository.ingestion.repository_url
    routing   = aws_ecr_repository.routing.repository_url
  }
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.main.name
}
