# ── Region ───────────────────────────────────────────────────
variable "aws_region" {
  type    = string
  default = "us-east-1"
}

# ── ECR & ECS settings (from HW5) ───────────────────────────
variable "ecr_repository_name" {
  type    = string
  default = "hw8_ecommerce"
}

variable "service_name" {
  type    = string
  default = "hw8-store"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "ecs_count" {
  type    = number
  default = 1
}

variable "log_retention_days" {
  type    = number
  default = 7
}

# ── HW8: RDS MySQL settings ─────────────────────────────────
variable "db_name" {
  description = "MySQL database name"
  type        = string
  default     = "ecommerce"
}

variable "db_username" {
  description = "MySQL master username"
  type        = string
  default     = "admin"
}

variable "db_password" {
  description = "MySQL master password"
  type        = string
  sensitive   = true
  # Set via: terraform apply -var="db_password=YourSecurePass123!"
  # Or via terraform.tfvars (gitignored)
}
