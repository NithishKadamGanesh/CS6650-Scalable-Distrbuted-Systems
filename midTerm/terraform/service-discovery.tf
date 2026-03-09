# =============================================================================
# AWS Cloud Map — Private DNS Service Discovery
# =============================================================================
# Creates "pizza.local" namespace in the VPC.
# Each downstream ECS service registers its task IPs here.
# Order API resolves: inventory.pizza.local, payment.pizza.local, etc.

resource "aws_service_discovery_private_dns_namespace" "main" {
  name = "pizza.local"
  vpc  = aws_vpc.main.id
  tags = { Name = "${var.project_name}-namespace" }
}

resource "aws_service_discovery_service" "downstream" {
  for_each = toset(local.all_services)
  name     = each.key

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.main.id
    dns_records {
      ttl  = 10
      type = "A"
    }
    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}
