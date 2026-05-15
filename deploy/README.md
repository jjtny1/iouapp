# Deploy IOU to AWS (ECS Fargate)

This runbook deploys the IOU bill-splitting app as a single Docker container on
AWS ECS Fargate, fronted by an HTTPS Application Load Balancer, with the SQLite
database persisted on EFS.

## Architecture

```
Internet
  → Route 53 (A alias record for your domain)
  → Application Load Balancer  (HTTPS :443 via ACM cert; HTTP :80 → 301 → HTTPS)
  → ECS Fargate service        (exactly 1 task, 256 CPU / 512 MiB, awsvpc,
                                public subnets, public IP — no NAT Gateway)
  → SQLite file on EFS         (mounted at /data, single-writer)
```

Design notes:

- **One task, always.** SQLite permits only one writer. The ECS service runs
  `desired_count = 1` with `deployment_minimum_healthy_percent = 0` and
  `deployment_maximum_percent = 100`, so a deploy stops the old task _before_
  starting the new one. There is never a moment with two writers.
- **No NAT Gateway.** The task runs in public subnets with a public IP so it
  can pull from ECR and reach the Anthropic and SES APIs directly. This saves
  roughly $32/month versus a NAT Gateway. Inbound access is still locked down:
  the service security group only accepts traffic from the ALB.
- **Non-root container.** The image is distroless `nonroot` (uid/gid 65532).
  The EFS access point is pinned to that uid/gid so the app can write `/data`.
- **Secret handling.** `ANTHROPIC_API_KEY` lives in SSM Parameter Store as a
  SecureString and is injected into the container by ECS. Terraform creates the
  parameter with a placeholder and never overwrites the value you set.

## Prerequisites

1. **Register the domain through Route 53 first** (AWS Console → Route 53 →
   Registered domains). Registering a domain auto-creates a public hosted zone
   for it. This Terraform config _looks up_ that zone with a data source — it
   does not create one. If your app domain is a subdomain (e.g.
   `iou.example.com` under an `example.com` zone), the `aws_route53_zone` data
   source resolves the closest matching zone.
2. **AWS credentials** with permissions to create VPC, ECS, ELB, ECR, EFS, IAM,
   ACM, Route 53, SES and SSM resources. Export them before running Terraform:
   ```bash
   export AWS_ACCESS_KEY_ID=...
   export AWS_SECRET_ACCESS_KEY=...
   export AWS_REGION=us-east-1
   ```
3. **Terraform** >= 1.5 and the **AWS CLI** and **Docker** installed locally.

## Order of operations

### 1. Register the domain

Do this in the AWS Console as described above. Wait until the domain shows as
registered and the hosted zone exists.

### 2. Configure variables

```bash
cd deploy/terraform
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars: set domain_name, mail_from_address, aws_region
```

### 3. Provision infrastructure

```bash
terraform init
terraform apply
```

The apply will:

- create the VPC, subnets, ALB, ECS cluster/service, ECR repo, EFS, IAM roles;
- request an ACM certificate and a SES domain identity, and write the Route 53
  DNS-validation and DKIM records automatically;
- block on `aws_acm_certificate_validation` until the certificate is issued
  (usually a few minutes).

On the first apply the ECS task will fail to start because no image has been
pushed to ECR yet — that is expected. Continue with the next steps.

### 4. Set the real Anthropic API key in SSM

Terraform created the SSM parameter with a placeholder. Set the real value:

```bash
aws ssm put-parameter \
  --name "/iou/ANTHROPIC_API_KEY" \
  --type SecureString \
  --value "sk-ant-..." \
  --overwrite
```

(The parameter name is also in the `ssm_anthropic_key_parameter` output.)
Terraform will not revert this — the resource has `ignore_changes = [value]`.

### 5. Build and push the Docker image to ECR

```bash
# from the repo root
ECR_URL=$(terraform -chdir=deploy/terraform output -raw ecr_repository_url)
# ECR URL looks like 123456789012.dkr.ecr.us-east-1.amazonaws.com/iou
REGION=$(echo "$ECR_URL" | cut -d. -f4)

aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "${ECR_URL%/*}"

docker build -t "$ECR_URL:latest" .
docker push "$ECR_URL:latest"
```

The container must listen on port 8080 and serve `GET /api/health` with a 200.

### 6. Roll out the image

```bash
CLUSTER=$(terraform -chdir=deploy/terraform output -raw ecs_cluster_name)
SERVICE=$(terraform -chdir=deploy/terraform output -raw ecs_service_name)

aws ecs update-service \
  --cluster "$CLUSTER" \
  --service "$SERVICE" \
  --force-new-deployment
```

Watch the deployment; the circuit breaker rolls back automatically if the new
task fails to become healthy. Once healthy, the app is live at the
`application_url` output (`https://<domain_name>`).

For every later release: push a new image tag (or overwrite `latest`) and run
the same `update-service --force-new-deployment` command.

## SES sandbox caveat

A brand-new AWS account has SES in **sandbox mode**: you can only send mail to
addresses/domains you have verified, and there is a low daily quota. IOU's
magic-link sign-in emails will therefore only reach verified recipients until
you leave the sandbox.

To send to arbitrary users, request production access:
AWS Console → SES → Account dashboard → _Request production access_. Until that
is granted, verify each test recipient under SES → Verified identities.

The sending **domain** identity and its DKIM records are created by this
Terraform; domain verification completes automatically once the Route 53 DKIM
CNAMEs propagate.

## Tear down

```bash
cd deploy/terraform
terraform destroy
```

Notes:

- `terraform destroy` deletes the EFS file system **and the SQLite database on
  it**. Back up `/data/iou.db` first if you need the data — e.g. by running an
  ECS exec into the task and copying the file out, or mounting the EFS access
  point from a temporary instance.
- The Route 53 **hosted zone is not managed by Terraform** and is left intact.
- The SSM parameter is destroyed; the secret value is lost.
- Destroy does not deregister the domain — do that separately in the console if
  desired.
