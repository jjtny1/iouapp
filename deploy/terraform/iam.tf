data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

data "aws_iam_policy_document" "ecs_tasks_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# --- Task execution role: used by the ECS agent to pull images, write logs,
#     and read the SSM secret when launching the task. ---
resource "aws_iam_role" "task_execution" {
  name               = "${var.project_name}-task-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume.json
}

resource "aws_iam_role_policy_attachment" "task_execution_managed" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "task_execution_secrets" {
  statement {
    sid       = "ReadAnthropicKey"
    actions   = ["ssm:GetParameters"]
    resources = [aws_ssm_parameter.anthropic_api_key.arn]
  }

  # SecureString parameters encrypted with the AWS-managed SSM key normally
  # need no explicit kms:Decrypt, but granting it on the alias is harmless and
  # covers the case where a customer-managed key is used.
  statement {
    sid       = "DecryptSsmSecret"
    actions   = ["kms:Decrypt"]
    resources = ["arn:aws:kms:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:alias/aws/ssm"]
  }
}

resource "aws_iam_role_policy" "task_execution_secrets" {
  name   = "${var.project_name}-task-execution-secrets"
  role   = aws_iam_role.task_execution.id
  policy = data.aws_iam_policy_document.task_execution_secrets.json
}

# --- Task role: assumed by the running container. Grants SES send. ---
resource "aws_iam_role" "task" {
  name               = "${var.project_name}-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume.json
}

data "aws_iam_policy_document" "task_ses" {
  statement {
    sid     = "SendEmail"
    actions = ["ses:SendEmail", "ses:SendRawEmail"]
    # Scoped to mail sent from the verified identity's domain.
    resources = ["*"]

    condition {
      test     = "StringEquals"
      variable = "ses:FromAddress"
      values   = [var.mail_from_address]
    }
  }
}

resource "aws_iam_role_policy" "task_ses" {
  name   = "${var.project_name}-task-ses"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task_ses.json
}
