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

# SecureString parameter holding the OpenAI API key, used for audio-split
# transcription (Whisper). Same out-of-band value pattern as the Anthropic
# key above: Terraform owns the resource with a placeholder, the operator
# sets the real value out-of-band, and ignore_changes keeps it.
resource "aws_ssm_parameter" "openai_api_key" {
  name        = "/${var.project_name}/OPENAI_API_KEY"
  description = "OpenAI API key for audio-split transcription. Real value set out-of-band."
  type        = "SecureString"
  value       = "PLACEHOLDER-set-real-value-out-of-band"

  lifecycle {
    ignore_changes = [value]
  }

  tags = {
    Name = "${var.project_name}-openai-api-key"
  }
}
