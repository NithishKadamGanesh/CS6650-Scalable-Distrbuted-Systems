# ===========================================================================
# IAM Roles for ECS
#
# AWS Academy / Learner Lab does NOT allow creating IAM roles.
# Instead, we use the pre-existing "LabRole" provided by the lab environment.
# ===========================================================================

data "aws_caller_identity" "current" {}

locals {
  lab_role_arn = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/LabRole"
}
