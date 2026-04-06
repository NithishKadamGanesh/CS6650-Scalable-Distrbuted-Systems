# ===========================================================================
# SNS Topic — order-processing-events
#
# SNS acts as the fan-out layer. When the order-receiver publishes an order,
# SNS delivers it to all subscribed endpoints (in this case, one SQS queue).
# This decouples publishers from consumers — you could later add a second
# queue for analytics, email notifications, etc. without changing the publisher.
# ===========================================================================

resource "aws_sns_topic" "order_events" {
  name = "order-processing-events"

  tags = {
    Name = "${var.project_name}-order-events"
  }
}

# ===========================================================================
# SQS Queue — order-processing-queue
#
# Configuration rationale:
#   visibility_timeout = 30s  → If a worker crashes mid-processing, the
#                                message reappears after 30s for retry.
#                                Must be > processing time (3s) to avoid
#                                duplicate processing during normal operation.
#   message_retention  = 4 days → Messages survive even if all workers are
#                                  down. Gives ops time to fix issues.
#   receive_wait_time  = 20s   → Long polling. Reduces empty responses and
#                                 API call costs. SQS holds the connection
#                                 open until a message arrives or timeout.
# ===========================================================================

resource "aws_sqs_queue" "order_queue" {
  name                       = "order-processing-queue"
  visibility_timeout_seconds = 30
  message_retention_seconds  = 345600 # 4 days
  receive_wait_time_seconds  = 20     # Long polling

  tags = {
    Name = "${var.project_name}-order-queue"
  }
}

# ===========================================================================
# SQS Queue Policy — Allow SNS to send messages to this queue
# ===========================================================================

resource "aws_sqs_queue_policy" "allow_sns" {
  queue_url = aws_sqs_queue.order_queue.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "AllowSNSPublish"
        Effect    = "Allow"
        Principal = { Service = "sns.amazonaws.com" }
        Action    = "sqs:SendMessage"
        Resource  = aws_sqs_queue.order_queue.arn
        Condition = {
          ArnEquals = {
            "aws:SourceArn" = aws_sns_topic.order_events.arn
          }
        }
      }
    ]
  })
}

# ===========================================================================
# SNS → SQS Subscription
#
# This wires the topic to the queue. Every message published to the SNS
# topic is delivered to the SQS queue. SNS wraps the original message in
# an envelope: {"Type": "Notification", "Message": "<your JSON>", ...}
# The order-processor must unwrap this envelope to get the Order JSON.
# ===========================================================================

resource "aws_sns_topic_subscription" "sqs_sub" {
  topic_arn = aws_sns_topic.order_events.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.order_queue.arn

  # raw_message_delivery = false (default) → SNS envelope wrapping
  # Set to true if you don't want the SNS envelope wrapper,
  # but the assignment expects the standard SNS→SQS pattern.
}
