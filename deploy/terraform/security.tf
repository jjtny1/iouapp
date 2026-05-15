# --- ALB security group: public HTTP/HTTPS ingress ---
resource "aws_security_group" "alb" {
  name        = "${var.project_name}-alb-sg"
  description = "Allow inbound HTTP/HTTPS to the load balancer"
  vpc_id      = aws_vpc.main.id

  tags = {
    Name = "${var.project_name}-alb-sg"
  }
}

resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTP from anywhere (redirected to HTTPS)"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "alb_https" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTPS from anywhere"
  cidr_ipv4         = "0.0.0.0/0"
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "alb_all" {
  security_group_id = aws_security_group.alb.id
  description       = "Allow all outbound"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

# --- ECS service security group: only the ALB may reach the app port ---
resource "aws_security_group" "service" {
  name        = "${var.project_name}-service-sg"
  description = "Allow inbound app traffic from the ALB only"
  vpc_id      = aws_vpc.main.id

  tags = {
    Name = "${var.project_name}-service-sg"
  }
}

resource "aws_vpc_security_group_ingress_rule" "service_from_alb" {
  security_group_id            = aws_security_group.service.id
  description                  = "App port 8080 from the ALB"
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "service_all" {
  security_group_id = aws_security_group.service.id
  description       = "Allow all outbound (ECR pulls, Anthropic + SES APIs)"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

# --- EFS security group: only the ECS service may reach NFS ---
resource "aws_security_group" "efs" {
  name        = "${var.project_name}-efs-sg"
  description = "Allow inbound NFS from the ECS service only"
  vpc_id      = aws_vpc.main.id

  tags = {
    Name = "${var.project_name}-efs-sg"
  }
}

resource "aws_vpc_security_group_ingress_rule" "efs_from_service" {
  security_group_id            = aws_security_group.efs.id
  description                  = "NFS 2049 from the ECS service"
  referenced_security_group_id = aws_security_group.service.id
  from_port                    = 2049
  to_port                      = 2049
  ip_protocol                  = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "efs_all" {
  security_group_id = aws_security_group.efs.id
  description       = "Allow all outbound"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}
