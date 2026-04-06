# =============================================================================
# HW8 Root — HW5 infra + RDS MySQL (Step I) + DynamoDB (Step II)
# =============================================================================

# ── Existing HW5 modules ────────────────────────────────────

module "network" {
  source         = "./modules/network"
  service_name   = var.service_name
  container_port = var.container_port
}

module "ecr" {
  source          = "./modules/ecr"
  repository_name = var.ecr_repository_name
}

module "logging" {
  source            = "./modules/logging"
  service_name      = var.service_name
  retention_in_days = var.log_retention_days
}

# Reuse existing LabRole for ECS tasks
data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

# ── Step I: RDS MySQL ────────────────────────────────────────

module "rds" {
  source = "./modules/rds"

  service_name          = var.service_name
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.subnet_ids
  ecs_security_group_id = module.network.security_group_id
  db_name               = var.db_name
  db_username           = var.db_username
  db_password           = var.db_password
}

# ── Step II: DynamoDB ────────────────────────────────────────

module "dynamodb" {
  source       = "./modules/dynamodb"
  service_name = var.service_name
}

# ── ECS (receives both MySQL + DynamoDB config) ──────────────

module "ecs" {
  source             = "./modules/ecs"
  service_name       = var.service_name
  image              = "${module.ecr.repository_url}:latest"
  container_port     = var.container_port
  subnet_ids         = module.network.subnet_ids
  security_group_ids = [module.network.security_group_id]
  execution_role_arn = data.aws_iam_role.lab_role.arn
  task_role_arn      = data.aws_iam_role.lab_role.arn
  log_group_name     = module.logging.log_group_name
  ecs_count          = var.ecs_count
  region             = var.aws_region

  # Step I: RDS connection details
  db_host     = module.rds.hostname
  db_port     = module.rds.port
  db_name     = module.rds.db_name
  db_user     = var.db_username
  db_password = var.db_password

  # Step II: DynamoDB table name
  dynamo_table_name = module.dynamodb.table_name
}

# ── Docker build & push to ECR ───────────────────────────────

resource "docker_image" "app" {
  name = "${module.ecr.repository_url}:latest"
  build {
    context = "../src"
  }
}

resource "docker_registry_image" "app" {
  name = docker_image.app.name
}
