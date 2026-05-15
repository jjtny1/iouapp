---
description: Verify an email address as an SES identity (sandbox whitelist)
---

!`if [ -z "$ARGUMENTS" ]; then echo "Usage: /add-ses <email>"; elif ! echo "$ARGUMENTS" | grep -Eq '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'; then echo "Not a valid email: $ARGUMENTS"; else aws ses verify-email-identity --email-address "$ARGUMENTS" --region us-east-1 && aws ses get-identity-verification-attributes --identities "$ARGUMENTS" --region us-east-1 --output json; fi`

The command above ran `verify-email-identity` for the address in `$ARGUMENTS`
(SES is in sandbox mode in `us-east-1`, account `283578588347`). AWS sends the
owner a verification email they must click before sign-in mail will deliver.

Then:

- If the output shows a usage/validation error, tell the user and stop.
- On success (status `Pending`), append the address to the pending-identities
  line in `~/.claude/projects/-Users-Satoshi-go-src-github-com-jjtny1-splitit/memory/project_aws_deploy.md`.
- Confirm to the user: address submitted, status `Pending`, owner must click
  the AWS verification email.
