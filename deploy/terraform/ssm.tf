# SecureString parameter holding the Anthropic API key.
#
# Terraform owns the resource but NOT the value: it is created with a
# placeholder and `ignore_changes = [value]` keeps Terraform from clobbering
# the real secret. The operator sets the real value out-of-band after the
# first apply (see deploy/README.md).
resource "aws_ssm_parameter" "anthropic_api_key" {
  name        = "/${var.project_name}/ANTHROPIC_API_KEY"
  description = "Anthropic API key for receipt parsing. Real value set out-of-band."
  type        = "SecureString"
  value       = "PLACEHOLDER-set-real-value-out-of-band"

  lifecycle {
    ignore_changes = [value]
  }

  tags = {
    Name = "${var.project_name}-anthropic-api-key"
  }
}
