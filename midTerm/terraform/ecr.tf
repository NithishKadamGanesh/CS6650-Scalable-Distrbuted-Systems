# =============================================================================
# ECR Repositories — one Docker image repo per service
# =============================================================================

resource "aws_ecr_repository" "order_api" {
  name                 = "${var.project_name}/order-api"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
  tags = { Name = "${var.project_name}-order-api" }
}

resource "aws_ecr_repository" "downstream" {
  for_each             = toset(local.all_services)
  name                 = "${var.project_name}/${each.key}"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
  tags = { Name = "${var.project_name}-${each.key}" }
}
