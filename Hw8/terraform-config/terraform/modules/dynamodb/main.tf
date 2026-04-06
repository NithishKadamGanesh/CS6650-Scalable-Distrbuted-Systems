# =============================================================================
# DynamoDB Module — Shopping Cart (NoSQL)
# =============================================================================
# Single-table design: cart metadata + items embedded in one document.
# Partition key: cart_id (Number) — auto-generated via atomic counter item.
# No sort key needed — all access patterns are by cart_id.
# On-demand billing (PAY_PER_REQUEST) — no capacity planning needed.
# =============================================================================

resource "aws_dynamodb_table" "shopping_carts" {
  name         = "${var.service_name}-dynamo-carts"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "cart_id"

  attribute {
    name = "cart_id"
    type = "N"
  }

  tags = {
    Name = "${var.service_name}-dynamo-carts"
  }
}
