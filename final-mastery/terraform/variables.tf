variable "aws_region" {
  description = "AWS region for deployment"
  type        = string
  default     = "us-west-2"
}

variable "project_name" {
  description = "Resource name prefix"
  type        = string
  default     = "final-mastery-album-store"
}

variable "image_tag" {
  description = "Container image tag pushed to ECR"
  type        = string
  default     = "latest"
}

variable "container_port" {
  description = "Port exposed by the Go service"
  type        = number
  default     = 8000
}

variable "task_cpu" {
  description = "Fargate task CPU units"
  type        = number
  default     = 2048
}

variable "task_memory" {
  description = "Fargate task memory in MiB"
  type        = number
  default     = 4096
}

variable "desired_count" {
  description = "Base ECS task count"
  type        = number
  default     = 6
}

variable "max_capacity" {
  description = "Maximum ECS task count under autoscaling"
  type        = number
  default     = 12
}

variable "max_workers" {
  description = "Background worker count for photo processing"
  type        = number
  default     = 32
}

variable "processing_delay_ms" {
  description = "Artificial processing delay; keep 0 for submission"
  type        = number
  default     = 0
}

variable "allowed_cidrs" {
  description = "CIDR blocks allowed to hit the ALB"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "execution_role_name" {
  description = "Existing IAM role name for ECS task execution in Learner Lab"
  type        = string
  default     = "LabRole"
}
