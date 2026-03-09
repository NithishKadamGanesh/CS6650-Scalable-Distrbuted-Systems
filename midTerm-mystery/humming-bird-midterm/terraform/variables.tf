variable "project_name" {
  description = "Base name used for AWS resources."
  type        = string
  default     = "hummingbird"
}

variable "environment" {
  description = "Deployment environment name (also used as NODE_ENV)."
  type        = string
  default     = "production"
}

variable "aws_region" {
  description = "AWS region to deploy into."
  type        = string
  default     = "us-west-2"
}

variable "aws_profile" {
  description = "Optional AWS CLI profile name for Terraform to use (useful when switching accounts). Leave empty to use the default credential chain."
  type        = string
  default     = ""
}

variable "app_port" {
  description = "Container/listener port the API listens on."
  type        = number
  default     = 9000
}

variable "image_uri" {
  description = "Full image URI to run in ECS (e.g. <acct>.dkr.ecr.<region>.amazonaws.com/hummingbird:latest)."
  type        = string
  default     = ""
}

variable "desired_count" {
  description = "Number of tasks to run."
  type        = number
  default     = 1
}

variable "cpu" {
  description = "Fargate task CPU units."
  type        = number
  default     = 256
}

variable "memory" {
  description = "Fargate task memory (MiB)."
  type        = number
  default     = 512
}

variable "ecs_cluster_name" {
  description = "ECS cluster name."
  type        = string
  default     = "humming-bird-cluster"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.10.0.0/16"
}

variable "public_subnet_cidrs" {
  description = "CIDR blocks for public subnets (must be 2+ for ALB HA)."
  type        = list(string)
  default     = ["10.10.1.0/24", "10.10.2.0/24"]
}

variable "allowed_ingress_cidrs" {
  description = "CIDRs allowed to access the public ALB (port 80)."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "bucket_name" {
  description = "S3 bucket name used for media storage (must be globally unique). If empty, Terraform will default to <project_name>-<environment>-<account_id>."
  type        = string
  default     = ""
}

variable "dynamodb_table_name" {
  description = "DynamoDB table name used to store media metadata."
  type        = string
  default     = "hummingbird-app-table"
}

variable "sns_topic_name" {
  description = "SNS topic name used for media management events."
  type        = string
  default     = "media-management-topic"
}

variable "use_existing_iam_roles" {
  description = "If true, Terraform will NOT create IAM roles/policies and will instead use existing IAM roles (useful for AWS Learner Lab LabRole)."
  type        = bool
  default     = false
}

variable "existing_task_role_name" {
  description = "Existing IAM role name to use as the ECS task role (app permissions). In Learner Lab this is commonly LabRole."
  type        = string
  default     = "LabRole"
}

variable "existing_execution_role_name" {
  description = "Existing IAM role name to use as the ECS task execution role (pull image, write logs). In Learner Lab this is commonly LabRole."
  type        = string
  default     = "LabRole"
}

variable "otel_enabled" {
  description = "Whether to enable OpenTelemetry exporting from the API container."
  type        = bool
  default     = false
}

variable "otel_exporter_otlp_endpoint" {
  description = "OTLP endpoint (e.g. http://collector:4318) when otel_enabled=true."
  type        = string
  default     = ""
}

variable "otel_exporter_otlp_headers" {
  description = "Optional OTLP headers, comma-separated (e.g. \"key1=value1,key2=value2\")."
  type        = string
  default     = ""
}

variable "honeycomb_enabled" {
  description = "Convenience toggle to export OpenTelemetry data to Honeycomb via OTLP/HTTP."
  type        = bool
  default     = false
}

variable "honeycomb_api_key" {
  description = "Honeycomb API key (sent as x-honeycomb-team header). Keep this out of version control."
  type        = string
  default     = ""
  sensitive   = true
}

variable "honeycomb_dataset" {
  description = "Honeycomb dataset name (sent as x-honeycomb-dataset header)."
  type        = string
  default     = "hummingbird"
}

variable "honeycomb_endpoint" {
  description = "Honeycomb OTLP endpoint base URL. Use https://api.honeycomb.io (US) or https://api.eu1.honeycomb.io (EU)."
  type        = string
  default     = "https://api.honeycomb.io"
}