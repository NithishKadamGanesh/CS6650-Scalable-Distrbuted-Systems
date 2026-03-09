# =============================================================================
# ECS Cluster, IAM, Task Definitions, Services, CloudWatch Logs
# =============================================================================

# ─── Cluster ───

resource "aws_ecs_cluster" "main" {
  name = "${var.project_name}-cluster"
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
  tags = { Name = "${var.project_name}-cluster" }
}

# ─── IAM Role for Task Execution ───

resource "aws_iam_role" "ecs_execution" {
  name = "${var.project_name}-ecs-execution"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_execution" {
  role       = aws_iam_role.ecs_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# ─── CloudWatch Log Groups ───

resource "aws_cloudwatch_log_group" "services" {
  for_each          = toset(concat(["order-api"], local.all_services))
  name              = "/ecs/${var.project_name}/${each.key}"
  retention_in_days = 7
}

# ─── Task Definition: Order API ───

resource "aws_ecs_task_definition" "order_api" {
  family                   = "${var.project_name}-order-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.ecs_execution.arn

  container_definitions = jsonencode([{
    name      = "order-api"
    image     = "${aws_ecr_repository.order_api.repository_url}:latest"
    essential = true
    portMappings = [{ containerPort = 8080, protocol = "tcp" }]
    environment = [
      { name = "PORT",          value = "8080" },
      { name = "MODE",          value = "none" },
      { name = "INVENTORY_URL", value = "http://inventory.pizza.local:8080" },
      { name = "PAYMENT_URL",   value = "http://payment.pizza.local:8080" },
      { name = "KITCHEN_URL",   value = "http://kitchen.pizza.local:8080" },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = "/ecs/${var.project_name}/order-api"
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

# ─── Task Definitions: Downstream Services ───

resource "aws_ecs_task_definition" "downstream" {
  for_each                 = var.services
  family                   = "${var.project_name}-${each.key}"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.ecs_execution.arn

  container_definitions = jsonencode([{
    name      = each.key
    image     = "${aws_ecr_repository.downstream[each.key].repository_url}:latest"
    essential = true
    portMappings = [{ containerPort = 8080, protocol = "tcp" }]
    environment = [
      { name = "SERVICE_NAME",  value = title(each.key) },
      { name = "BASE_DELAY_MS", value = tostring(each.value.base_delay_ms) },
      { name = "PORT",          value = "8080" },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = "/ecs/${var.project_name}/${each.key}"
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

# ─── ECS Service: Order API (behind ALB) ───

resource "aws_ecs_service" "order_api" {
  name            = "${var.project_name}-order-api"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.order_api.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.order_api.arn
    container_name   = "order-api"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.http]
}

# ─── ECS Services: Downstream (registered with Cloud Map) ───

resource "aws_ecs_service" "downstream" {
  for_each        = var.services
  name            = "${var.project_name}-${each.key}"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.downstream[each.key].arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = true
  }

  service_registries {
    registry_arn = aws_service_discovery_service.downstream[each.key].arn
  }
}
