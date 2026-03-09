variable "aws_region" {
  description = "AWS region for all resources"
  default     = "us-east-1"
}

variable "project_name" {
  description = "Project name — used as prefix for all resource names"
  default     = "galactic-pizza"
}

variable "services" {
  description = "Downstream services and their simulated processing delay"
  default = {
    inventory = { base_delay_ms = 30 }
    payment   = { base_delay_ms = 60 }
    kitchen   = { base_delay_ms = 40 }
  }
}

locals {
  all_services = keys(var.services)
  azs          = slice(data.aws_availability_zones.available.names, 0, 2)
}
