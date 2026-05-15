variable "aws_region" {
  description = "AWS region to deploy all resources into."
  type        = string
  default     = "us-east-1"
}

variable "domain_name" {
  description = "Fully-qualified domain name the app is served at (e.g. iou.example.com). A Route 53 hosted zone for this domain (or its parent) must already exist."
  type        = string
}

variable "mail_from_address" {
  description = "The From address used for outbound mail via SES (e.g. no-reply@iou.example.com). Its domain must be covered by the SES email identity."
  type        = string
}

variable "app_image_tag" {
  description = "The ECR image tag the ECS task definition runs."
  type        = string
  default     = "latest"
}

variable "project_name" {
  description = "Short project name used for resource naming and the Project tag."
  type        = string
  default     = "iou"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.0.0.0/16"
}

variable "task_cpu" {
  description = "Fargate task CPU units (256 = 0.25 vCPU)."
  type        = number
  default     = 256
}

variable "task_memory" {
  description = "Fargate task memory in MiB."
  type        = number
  default     = 512
}
