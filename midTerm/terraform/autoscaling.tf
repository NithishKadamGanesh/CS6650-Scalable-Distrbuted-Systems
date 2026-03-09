# =============================================================================
# Auto-Scaling — CPU Target Tracking at 70% (same approach as HW6)
# =============================================================================

# ─── Order API: scale 2 → 6 ───

resource "aws_appautoscaling_target" "order_api" {
  max_capacity       = 6
  min_capacity       = 2
  resource_id        = "service/${aws_ecs_cluster.main.name}/${aws_ecs_service.order_api.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "order_api_cpu" {
  name               = "${var.project_name}-order-api-cpu"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.order_api.resource_id
  scalable_dimension = aws_appautoscaling_target.order_api.scalable_dimension
  service_namespace  = aws_appautoscaling_target.order_api.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 70.0
    scale_in_cooldown  = 60
    scale_out_cooldown = 30
  }
}

# ─── Downstream Services: scale 2 → 4 each ───

resource "aws_appautoscaling_target" "downstream" {
  for_each           = var.services
  max_capacity       = 4
  min_capacity       = 2
  resource_id        = "service/${aws_ecs_cluster.main.name}/${aws_ecs_service.downstream[each.key].name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "downstream_cpu" {
  for_each           = var.services
  name               = "${var.project_name}-${each.key}-cpu"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.downstream[each.key].resource_id
  scalable_dimension = aws_appautoscaling_target.downstream[each.key].scalable_dimension
  service_namespace  = aws_appautoscaling_target.downstream[each.key].service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 70.0
    scale_in_cooldown  = 60
    scale_out_cooldown = 30
  }
}
