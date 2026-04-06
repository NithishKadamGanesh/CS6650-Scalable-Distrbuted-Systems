# ===========================================================================
# Lambda Function - Serverless Order Processor (Part III)
#
# Instead of SNS -> SQS -> ECS Workers, this goes:
#   SNS -> Lambda (AWS manages everything)
#
# Same 3-second payment processing, zero operational overhead.
# No queues to monitor, no workers to scale, no 3am alerts.
# ===========================================================================

# Zip file containing the compiled Go binary
# Must be built before terraform apply (see build-lambda.ps1)
resource "aws_lambda_function" "order_processor" {
  filename         = "${path.module}/../order-lambda/bootstrap.zip"
  function_name    = "${var.project_name}-order-lambda"
  role             = local.lab_role_arn
  handler          = "bootstrap"
  runtime          = "provided.al2"
  memory_size      = 512
  timeout          = 30

  source_code_hash = filebase64sha256("${path.module}/../order-lambda/bootstrap.zip")

  environment {
    variables = {
      AWS_LAMBDA_EXEC_WRAPPER = ""
    }
  }

  tags = {
    Name = "${var.project_name}-order-lambda"
  }
}

# ===========================================================================
# SNS -> Lambda Subscription
#
# Lambda subscribes directly to the SNS topic - no SQS queue in between.
# When an order is published to SNS, Lambda is invoked immediately.
#
# Trade-off vs SQS:
#   - No message persistence (if Lambda fails, SNS retries 2x then discards)
#   - No visibility timeout tuning
#   - No queue depth monitoring
#   - But also no retry control and no batch processing
# ===========================================================================

resource "aws_sns_topic_subscription" "lambda_sub" {
  topic_arn = aws_sns_topic.order_events.arn
  protocol  = "lambda"
  endpoint  = aws_lambda_function.order_processor.arn
}

# ===========================================================================
# Permission for SNS to invoke the Lambda function
# ===========================================================================

resource "aws_lambda_permission" "sns_invoke" {
  statement_id  = "AllowSNSInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.order_processor.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.order_events.arn
}

# ===========================================================================
# CloudWatch Log Group for Lambda
# ===========================================================================

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${var.project_name}-order-lambda"
  retention_in_days = 7
}
