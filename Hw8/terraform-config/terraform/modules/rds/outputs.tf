output "endpoint" {
  description = "RDS instance endpoint (hostname:port)"
  value       = aws_db_instance.mysql.endpoint
}

output "hostname" {
  description = "RDS instance hostname only"
  value       = aws_db_instance.mysql.address
}

output "port" {
  description = "RDS instance port"
  value       = aws_db_instance.mysql.port
}

output "db_name" {
  description = "Database name"
  value       = aws_db_instance.mysql.db_name
}

output "security_group_id" {
  description = "Security group ID of the RDS instance"
  value       = aws_security_group.mysql.id
}
