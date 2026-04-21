variable "region" {
  type        = string
  default     = "us-east-1"
  description = "AWS region"
}

variable "db_password" {
  type        = string
  sensitive   = true
  description = "Postgres master password — set in terraform.tfvars, DO NOT COMMIT"
}

variable "routing_task_count" {
  type        = number
  default     = 4
  description = "Initial desired count for routing service (varied in Experiment 1)"
}

variable "execution_role_name" {
  type        = string
  default     = "LabRole"
  description = "Existing IAM role name to use for ECS task execution in learner-lab style accounts"
}

variable "task_role_name" {
  type        = string
  default     = ""
  description = "Optional existing IAM role name for ECS task permissions; defaults to execution_role_name"
}
