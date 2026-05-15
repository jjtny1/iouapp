locals {
  # Sending domain is the host portion of the From address
  # (e.g. no-reply@iou.example.com -> iou.example.com).
  mail_domain = element(split("@", var.mail_from_address), 1)
}

# Verified SES email identity for the sending domain, with DKIM signing.
resource "aws_sesv2_email_identity" "sending" {
  email_identity = local.mail_domain

  dkim_signing_attributes {
    next_signing_key_length = "RSA_2048_BIT"
  }

  tags = {
    Name = "${var.project_name}-ses-identity"
  }
}

# Route 53 CNAME records that prove DKIM ownership of the sending domain.
resource "aws_route53_record" "ses_dkim" {
  count = 3

  zone_id = data.aws_route53_zone.main.zone_id
  name    = "${aws_sesv2_email_identity.sending.dkim_signing_attributes[0].tokens[count.index]}._domainkey.${local.mail_domain}"
  type    = "CNAME"
  ttl     = 600
  records = ["${aws_sesv2_email_identity.sending.dkim_signing_attributes[0].tokens[count.index]}.dkim.amazonses.com"]
}
