output "alb_dns_name" {
  description = "Public DNS name of the Application Load Balancer."
  value       = aws_lb.main.dns_name
}

output "ecr_repository_url" {
  description = "ECR repository URL to push the application image to."
  value       = aws_ecr_repository.app.repository_url
}

output "application_url" {
  description = "Public HTTPS URL of the application."
  value       = "https://${var.domain_name}"
}

output "ecs_cluster_name" {
  description = "Name of the ECS cluster."
  value       = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  description = "Name of the ECS service."
  value       = aws_ecs_service.app.name
}

output "ssm_anthropic_key_parameter" {
  description = "Name of the SSM SecureString parameter to set the real Anthropic API key into."
  value       = aws_ssm_parameter.anthropic_api_key.name
}
