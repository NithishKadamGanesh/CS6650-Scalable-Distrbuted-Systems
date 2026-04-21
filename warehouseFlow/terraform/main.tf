# WarehouseFlow — Full AWS Infrastructure
# Provisions: VPC + subnets, ECS Fargate cluster with ALB,
# MSK (Kafka), 3x ElastiCache (Redis), RDS (Postgres), SQS DLQ, ECR.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.40" }
  }
}

provider "aws" {
  region = var.region
}

data "aws_iam_role" "ecs_exec" {
  name = var.execution_role_name
}

data "aws_iam_role" "ecs_task" {
  name = var.task_role_name != "" ? var.task_role_name : var.execution_role_name
}

locals {
  name = "warehouseflow"
  tags = {
    Project = "WarehouseFlow"
    Course  = "CS6650"
    Owner   = "Nithish"
  }
}

# ─── VPC + networking ────────────────────────────────────────────────────────
resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  tags                 = merge(local.tags, { Name = "${local.name}-vpc" })
}

resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.main.id
  tags   = merge(local.tags, { Name = "${local.name}-igw" })
}

resource "aws_subnet" "public" {
  count                   = 2
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.0.${count.index + 1}.0/24"
  availability_zone       = "${var.region}${["a", "b"][count.index]}"
  map_public_ip_on_launch = true
  tags                    = merge(local.tags, { Name = "${local.name}-public-${count.index}" })
}

resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.${count.index + 10}.0/24"
  availability_zone = "${var.region}${["a", "b"][count.index]}"
  tags              = merge(local.tags, { Name = "${local.name}-private-${count.index}" })
}

resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = merge(local.tags, { Name = "${local.name}-nat-eip" })
}

resource "aws_nat_gateway" "nat" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id
  tags          = merge(local.tags, { Name = "${local.name}-nat" })
  depends_on    = [aws_internet_gateway.igw]
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
}
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat.id
  }
}
resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}
resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# ─── Security groups ─────────────────────────────────────────────────────────
resource "aws_security_group" "alb" {
  name   = "${local.name}-alb-sg"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.tags
}

resource "aws_security_group" "ecs" {
  name   = "${local.name}-ecs-sg"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port       = 8081
    to_port         = 8082
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.tags
}

resource "aws_security_group" "data" {
  name   = "${local.name}-data-sg"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    security_groups = [aws_security_group.ecs.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.tags
}

# ─── ECR ─────────────────────────────────────────────────────────────────────
resource "aws_ecr_repository" "ingestion" {
  name                 = "${local.name}-ingestion"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
  tags                 = local.tags
}
resource "aws_ecr_repository" "routing" {
  name                 = "${local.name}-routing"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
  tags                 = local.tags
}

# ─── MSK (Kafka) ─────────────────────────────────────────────────────────────
resource "aws_msk_cluster" "main" {
  cluster_name           = "${local.name}-msk"
  kafka_version          = "3.6.0"
  number_of_broker_nodes = 2

  broker_node_group_info {
    instance_type   = "kafka.t3.small"
    client_subnets  = aws_subnet.private[*].id
    security_groups = [aws_security_group.data.id]
    storage_info {
      ebs_storage_info { volume_size = 20 }
    }
  }
  tags = local.tags
}

# ─── 3 ElastiCache clusters — one per warehouse ──────────────────────────────
resource "aws_elasticache_subnet_group" "main" {
  name       = "${local.name}-cache-subnets"
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_elasticache_cluster" "warehouse" {
  for_each             = toset(["a", "b", "c"])
  cluster_id           = "${local.name}-${each.value}"
  engine               = "redis"
  node_type            = "cache.t3.micro"
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  subnet_group_name    = aws_elasticache_subnet_group.main.name
  security_group_ids   = [aws_security_group.data.id]
  tags                 = merge(local.tags, { WarehouseID = each.value })
}

# ─── RDS (Postgres) ──────────────────────────────────────────────────────────
resource "aws_db_subnet_group" "main" {
  name       = "${local.name}-db-subnets"
  subnet_ids = aws_subnet.private[*].id
  tags       = local.tags
}

resource "aws_db_instance" "main" {
  identifier             = "${local.name}-pg"
  engine                 = "postgres"
  engine_version         = "16"
  instance_class         = "db.t3.micro"
  allocated_storage      = 20
  db_name                = "warehouseflow"
  username               = "warehouseflow"
  password               = var.db_password
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.data.id]
  skip_final_snapshot    = true
  publicly_accessible    = false
  tags                   = local.tags
}

# ─── SQS DLQ ─────────────────────────────────────────────────────────────────
resource "aws_sqs_queue" "dlq" {
  name                      = "${local.name}-dlq"
  message_retention_seconds = 345600
  tags                      = local.tags
}

# ─── ALB ────────────────────────────────────────────────────────────────────
resource "aws_lb" "main" {
  name               = "${local.name}-alb"
  load_balancer_type = "application"
  subnets            = aws_subnet.public[*].id
  security_groups    = [aws_security_group.alb.id]
  tags               = local.tags
}

resource "aws_lb_target_group" "ingestion" {
  name        = "${local.name}-ingestion-tg"
  port        = 8081
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"
  health_check {
    path                = "/health"
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
  tags = local.tags
}

resource "aws_lb_listener" "main" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.ingestion.arn
  }
}

# ─── ECS cluster ─────────────────────────────────────────────────────────────
resource "aws_ecs_cluster" "main" {
  name = "${local.name}-cluster"
  tags = local.tags
}

resource "aws_cloudwatch_log_group" "ingestion" {
  name              = "/ecs/${local.name}-ingestion"
  retention_in_days = 7
}

resource "aws_cloudwatch_log_group" "routing" {
  name              = "/ecs/${local.name}-routing"
  retention_in_days = 7
}

resource "aws_iam_role" "ecs_exec" {
  count = 0
  name  = "${local.name}-ecs-exec"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_exec" {
  count      = 0
  role       = "disabled"
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role" "ecs_task" {
  count = 0
  name  = "${local.name}-ecs-task"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy" "ecs_task_sqs" {
  count = 0
  name  = "${local.name}-sqs-access"
  role  = "disabled"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["sqs:SendMessage", "sqs:GetQueueAttributes"]
      Resource = aws_sqs_queue.dlq.arn
    }]
  })
}

# ─── ECS services (ingestion + routing) ──────────────────────────────────────
resource "aws_ecs_task_definition" "ingestion" {
  family                   = "${local.name}-ingestion"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "512"
  memory                   = "1024"
  execution_role_arn       = data.aws_iam_role.ecs_exec.arn
  task_role_arn            = data.aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name         = "ingestion"
    image        = "${aws_ecr_repository.ingestion.repository_url}:latest"
    essential    = true
    portMappings = [{ containerPort = 8081 }]
    environment = [
      { name = "KAFKA_BROKER", value = aws_msk_cluster.main.bootstrap_brokers },
      { name = "KAFKA_TOPIC", value = "orders" },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.ingestion.name
        "awslogs-region"        = var.region
        "awslogs-stream-prefix" = "ingestion"
      }
    }
  }])
}

resource "aws_ecs_task_definition" "routing" {
  family                   = "${local.name}-routing"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "1024"
  memory                   = "2048"
  execution_role_arn       = data.aws_iam_role.ecs_exec.arn
  task_role_arn            = data.aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name         = "routing"
    image        = "${aws_ecr_repository.routing.repository_url}:latest"
    essential    = true
    portMappings = [{ containerPort = 8082 }]
    environment = [
      { name = "KAFKA_BROKER", value = aws_msk_cluster.main.bootstrap_brokers },
      { name = "KAFKA_TOPIC", value = "orders" },
      { name = "REDIS_ADDR_A", value = "${aws_elasticache_cluster.warehouse["a"].cache_nodes[0].address}:6379" },
      { name = "REDIS_ADDR_B", value = "${aws_elasticache_cluster.warehouse["b"].cache_nodes[0].address}:6379" },
      { name = "REDIS_ADDR_C", value = "${aws_elasticache_cluster.warehouse["c"].cache_nodes[0].address}:6379" },
      { name = "POSTGRES_DSN", value = "postgres://warehouseflow:${var.db_password}@${aws_db_instance.main.endpoint}/warehouseflow?sslmode=require" },
      { name = "SQS_QUEUE_URL", value = aws_sqs_queue.dlq.url },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.routing.name
        "awslogs-region"        = var.region
        "awslogs-stream-prefix" = "routing"
      }
    }
  }])
}

resource "aws_ecs_service" "ingestion" {
  name            = "${local.name}-ingestion"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.ingestion.arn
  desired_count   = 2
  launch_type     = "FARGATE"
  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.ingestion.arn
    container_name   = "ingestion"
    container_port   = 8081
  }
}

resource "aws_ecs_service" "routing" {
  name            = "${local.name}-routing"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.routing.arn
  desired_count   = var.routing_task_count
  launch_type     = "FARGATE"
  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }
}

# ─── Auto-scaling — Experiment 7 uses this to trigger cold starts ───────────
resource "aws_appautoscaling_target" "routing" {
  max_capacity       = 12
  min_capacity       = 2
  resource_id        = "service/${aws_ecs_cluster.main.name}/${aws_ecs_service.routing.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "routing_cpu" {
  name               = "${local.name}-routing-cpu-scale"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.routing.resource_id
  scalable_dimension = aws_appautoscaling_target.routing.scalable_dimension
  service_namespace  = aws_appautoscaling_target.routing.service_namespace

  target_tracking_scaling_policy_configuration {
    target_value = 70.0
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
  }
}
