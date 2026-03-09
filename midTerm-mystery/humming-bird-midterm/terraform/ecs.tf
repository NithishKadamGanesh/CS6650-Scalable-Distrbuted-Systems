resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.project_name}/${var.environment}/api"
  retention_in_days = 14

  tags = {
    Project     = var.project_name
    Environment = var.environment
  }
}

resource "aws_ecs_cluster" "this" {
  name = local.ecs_cluster_name

  tags = {
    Project     = var.project_name
    Environment = var.environment
  }
}

locals {
  ecs_cluster_name   = var.ecs_cluster_name
  execution_role_arn = var.use_existing_iam_roles ? data.aws_iam_role.existing_execution[0].arn : aws_iam_role.ecs_task_execution[0].arn
  task_role_arn      = var.use_existing_iam_roles ? data.aws_iam_role.existing_task[0].arn : aws_iam_role.ecs_task[0].arn

  otel_enabled_effective  = var.otel_enabled || var.honeycomb_enabled
  otel_endpoint_effective = var.honeycomb_enabled ? var.honeycomb_endpoint : var.otel_exporter_otlp_endpoint
  otel_headers_effective  = var.honeycomb_enabled ? "x-honeycomb-team=${var.honeycomb_api_key},x-honeycomb-dataset=${var.honeycomb_dataset}" : var.otel_exporter_otlp_headers

  otel_env = local.otel_enabled_effective ? concat([
    { name = "OTEL_SDK_DISABLED", value = "false" },
    { name = "OTEL_TRACES_EXPORTER", value = "otlp" },
    { name = "OTEL_METRICS_EXPORTER", value = "otlp" },
    { name = "OTEL_EXPORTER_OTLP_PROTOCOL", value = "http/protobuf" },
    { name = "OTEL_EXPORTER_OTLP_ENDPOINT", value = local.otel_endpoint_effective }
    ], local.otel_headers_effective != "" ? [
    { name = "OTEL_EXPORTER_OTLP_HEADERS", value = local.otel_headers_effective }
    ] : []) : [
    { name = "OTEL_SDK_DISABLED", value = "true" },
    { name = "OTEL_TRACES_EXPORTER", value = "none" },
    { name = "OTEL_METRICS_EXPORTER", value = "none" }
  ]
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.project_name}-${var.environment}-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = local.execution_role_arn
  task_role_arn            = local.task_role_arn

  container_definitions = jsonencode([
    {
      name      = "api"
      image     = local.image_uri_effective
      essential = true
      portMappings = [
        {
          containerPort = var.app_port
          protocol      = "tcp"
        }
      ]
      environment = concat([
        { name = "APP_PORT", value = tostring(var.app_port) },
        { name = "AWS_REGION", value = var.aws_region },
        { name = "MEDIA_BUCKET_NAME", value = local.media_bucket_name },
        { name = "MEDIA_DYNAMODB_TABLE_NAME", value = aws_dynamodb_table.media.name },
        { name = "MEDIA_MANAGEMENT_TOPIC_ARN", value = aws_sns_topic.media_management.arn },
        { name = "NODE_ENV", value = var.environment }
      ], local.otel_env)
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.api.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "api"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "api" {
  name            = "${var.project_name}-${var.environment}-api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = [for s in aws_subnet.public : s.id]
    security_groups  = [aws_security_group.task.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = var.app_port
  }

  depends_on = [aws_lb_listener.http]

  # Learner-lab friendly: don't block `terraform apply` waiting for tasks to become healthy.
  # If the image tag isn't pushed yet, ECS may show image-pull errors until you push to ECR.
  wait_for_steady_state = false
}

