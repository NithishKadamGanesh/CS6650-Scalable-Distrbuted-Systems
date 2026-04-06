output "ecs_cluster_name" {
  description = "Name of the created ECS cluster"
  value       = module.ecs.cluster_name
}

output "ecs_service_name" {
  description = "Name of the running ECS service"
  value       = module.ecs.service_name
}

# Step I: RDS outputs
output "rds_endpoint" {
  description = "RDS MySQL endpoint (host:port)"
  value       = module.rds.endpoint
}

output "rds_hostname" {
  description = "RDS MySQL hostname"
  value       = module.rds.hostname
}

output "rds_db_name" {
  description = "Database name"
  value       = module.rds.db_name
}

# Step II: DynamoDB outputs
output "dynamodb_table_name" {
  description = "DynamoDB shopping carts table name"
  value       = module.dynamodb.table_name
}
