variable "service_name" {
  description = "Project name prefix for resource naming"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID where RDS will be deployed"
  type        = string
}

variable "subnet_ids" {
  description = "List of subnet IDs for DB subnet group (need ≥2 in different AZs)"
  type        = list(string)
}

variable "ecs_security_group_id" {
  description = "Security group ID of ECS tasks (allowed to connect to MySQL)"
  type        = string
}

variable "db_name" {
  description = "Name of the MySQL database to create"
  type        = string
  default     = "ecommerce"
}

variable "db_username" {
  description = "Master username for the database"
  type        = string
  default     = "admin"
}

variable "db_password" {
  description = "Master password for the database"
  type        = string
  sensitive   = true
}
