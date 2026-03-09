data "aws_iam_policy_document" "ecs_task_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

data "aws_iam_role" "existing_task" {
  count = var.use_existing_iam_roles ? 1 : 0
  name  = var.existing_task_role_name
}

data "aws_iam_role" "existing_execution" {
  count = var.use_existing_iam_roles ? 1 : 0
  name  = var.existing_execution_role_name
}

resource "aws_iam_role" "ecs_task_execution" {
  count              = var.use_existing_iam_roles ? 0 : 1
  name               = "${var.project_name}-${var.environment}-ecs-task-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json
}

resource "aws_iam_role_policy_attachment" "ecs_task_execution" {
  count      = var.use_existing_iam_roles ? 0 : 1
  role       = aws_iam_role.ecs_task_execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role" "ecs_task" {
  count              = var.use_existing_iam_roles ? 0 : 1
  name               = "${var.project_name}-${var.environment}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json
}

data "aws_iam_policy_document" "ecs_task_inline" {
  statement {
    sid    = "S3Objects"
    effect = "Allow"
    actions = [
      "s3:PutObject",
      "s3:GetObject",
      "s3:DeleteObject",
      "s3:CopyObject"
    ]
    resources = ["${aws_s3_bucket.media.arn}/*"]
  }

  statement {
    sid    = "DynamoDBTable"
    effect = "Allow"
    actions = [
      "dynamodb:PutItem",
      "dynamodb:GetItem",
      "dynamodb:UpdateItem",
      "dynamodb:DeleteItem"
    ]
    resources = [aws_dynamodb_table.media.arn]
  }

  statement {
    sid       = "SNSPublish"
    effect    = "Allow"
    actions   = ["sns:Publish"]
    resources = [aws_sns_topic.media_management.arn]
  }

  statement {
    sid    = "SqsConsume"
    effect = "Allow"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
      "sqs:ChangeMessageVisibility"
    ]
    resources = [aws_sqs_queue.media_events.arn]
  }
}

resource "aws_iam_role_policy" "ecs_task_inline" {
  count  = var.use_existing_iam_roles ? 0 : 1
  name   = "${var.project_name}-${var.environment}-ecs-task-inline"
  role   = aws_iam_role.ecs_task[0].id
  policy = data.aws_iam_policy_document.ecs_task_inline.json
}

