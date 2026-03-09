resource "aws_sqs_queue" "media_events" {
  name                       = "${var.project_name}-${var.environment}-media-events"
  message_retention_seconds  = 1209600
  visibility_timeout_seconds = 60
  receive_wait_time_seconds  = 20

  tags = {
    Project     = var.project_name
    Environment = var.environment
  }
}

data "aws_iam_policy_document" "media_events_queue_policy" {
  statement {
    sid     = "AllowSnsPublish"
    effect  = "Allow"
    actions = ["sqs:SendMessage"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    resources = [aws_sqs_queue.media_events.arn]

    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = [aws_sns_topic.media_management.arn]
    }
  }
}

resource "aws_sqs_queue_policy" "media_events" {
  queue_url = aws_sqs_queue.media_events.id
  policy    = data.aws_iam_policy_document.media_events_queue_policy.json
}

resource "aws_sns_topic_subscription" "media_events_sqs" {
  topic_arn            = aws_sns_topic.media_management.arn
  protocol             = "sqs"
  endpoint             = aws_sqs_queue.media_events.arn
  raw_message_delivery = true
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "${var.project_name}-${var.environment}-worker"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = local.execution_role_arn
  task_role_arn            = local.task_role_arn

  container_definitions = jsonencode([
    {
      name      = "worker"
      image     = local.image_uri_effective
      essential = true
      command   = ["node", "worker/processor.js"]
      environment = concat([
        { name = "AWS_REGION", value = var.aws_region },
        { name = "MEDIA_BUCKET_NAME", value = local.media_bucket_name },
        { name = "MEDIA_DYNAMODB_TABLE_NAME", value = aws_dynamodb_table.media.name },
        { name = "MEDIA_QUEUE_URL", value = aws_sqs_queue.media_events.id },
        { name = "NODE_ENV", value = var.environment }
      ], local.otel_env)
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.api.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "worker"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "worker" {
  name            = "${var.project_name}-${var.environment}-worker"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = [for s in aws_subnet.public : s.id]
    security_groups  = [aws_security_group.task.id]
    assign_public_ip = true
  }

  wait_for_steady_state = false
}

