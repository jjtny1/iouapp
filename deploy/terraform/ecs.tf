resource "aws_ecs_cluster" "main" {
  name = "${var.project_name}-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = {
    Name = "${var.project_name}-cluster"
  }
}

resource "aws_cloudwatch_log_group" "app" {
  name              = "/ecs/${var.project_name}"
  retention_in_days = 30

  tags = {
    Name = "/ecs/${var.project_name}"
  }
}

resource "aws_ecs_task_definition" "app" {
  family                   = var.project_name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.task_cpu
  memory                   = var.task_memory
  execution_role_arn       = aws_iam_role.task_execution.arn
  task_role_arn            = aws_iam_role.task.arn

  # EFS volume for the SQLite database, accessed through the uid/gid-pinned
  # access point.
  volume {
    name = "data"

    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.data.id
      transit_encryption = "ENABLED"

      authorization_config {
        access_point_id = aws_efs_access_point.data.id
        iam             = "DISABLED"
      }
    }
  }

  container_definitions = jsonencode([
    {
      name      = var.project_name
      image     = "${aws_ecr_repository.app.repository_url}:${var.app_image_tag}"
      essential = true

      portMappings = [
        {
          containerPort = 8080
          protocol      = "tcp"
        }
      ]

      # IOU_DEV is intentionally NOT set -> production behaviour.
      environment = [
        { name = "PORT", value = "8080" },
        { name = "IOU_DB", value = "/data/iou.db" },
        { name = "IOU_BASE_URL", value = "https://${var.domain_name}" },
        { name = "IOU_MAIL_PROVIDER", value = "ses" },
        { name = "IOU_MAIL_FROM", value = var.mail_from_address },
        { name = "AWS_REGION", value = var.aws_region },
      ]

      secrets = [
        {
          name      = "ANTHROPIC_API_KEY"
          valueFrom = aws_ssm_parameter.anthropic_api_key.arn
        },
        {
          name      = "OPENAI_API_KEY"
          valueFrom = aws_ssm_parameter.openai_api_key.arn
        }
      ]

      mountPoints = [
        {
          sourceVolume  = "data"
          containerPath = "/data"
          readOnly      = false
        }
      ]

      # Health is gated by the ALB target-group health check (path
      # /api/health). A container-level `healthCheck` is intentionally NOT
      # set: a distroless image has no shell and no wget/curl, so a CMD-SHELL
      # probe cannot run. If the app image is later changed to bundle a
      # health-probe binary, add a `healthCheck` block here.

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.app.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])

  tags = {
    Name = "${var.project_name}-task"
  }
}

resource "aws_ecs_service" "app" {
  name            = "${var.project_name}-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  # SQLite allows a single writer: never run two tasks at once. Stop the old
  # task before starting the new one on every deploy.
  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.service.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = var.project_name
    container_port   = 8080
  }

  # Give the task time to come up before health checks count against it.
  health_check_grace_period_seconds = 60

  depends_on = [
    aws_lb_listener.https,
    aws_efs_mount_target.data,
  ]

  tags = {
    Name = "${var.project_name}-service"
  }
}
