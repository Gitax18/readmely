# Deploying the backend on AWS

The web app runs on Vercel. This guide puts everything else on **one EC2 instance** with Docker
Compose: the Go API, the analysis worker, the AI service and worker, Postgres (pgvector) and Redis.
It is sized to run on AWS promotional credits, with alerts that fire long before a real charge.

Every step lists the AWS CLI command; the console path is given where a console click is simpler.
Do the steps in order. Expect about an hour the first time.

- [How it fits together](#how-it-fits-together)
- [What it costs](#what-it-costs)
- [0. Secure the account](#0-secure-the-account)
- [1. Billing alerts](#1-billing-alerts)
- [2. Shell variables for the rest of this guide](#2-shell-variables-for-the-rest-of-this-guide)
- [3. Storage: ECR repositories and the ops bucket](#3-storage-ecr-repositories-and-the-ops-bucket)
- [4. Network: security group and Elastic IP](#4-network-security-group-and-elastic-ip)
- [5. IAM roles](#5-iam-roles)
- [6. Secrets and settings](#6-secrets-and-settings)
- [7. Launch the instance](#7-launch-the-instance)
- [8. First deploy](#8-first-deploy)
- [9. Point Vercel at the API](#9-point-vercel-at-the-api)
- [10. Third-party services](#10-third-party-services)
- [11. Monitoring](#11-monitoring)
- [Operations](#operations)
- [Security checklist](#security-checklist)
- [Tearing it all down](#tearing-it-all-down)

## How it fits together

```
Browser ─► Vercel (Next.js) ──HTTPS + X-Readmely-Origin-Key──► EC2 t3.small
                                                               └─ Caddy :443 (TLS, origin-key gate)
                                                                   ├─ /healthz      → api:8080 (open)
                                                                   ├─ /_internal/*  → api:8081 (status detail)
                                                                   └─ everything    → api:8080
                                                                  api · ingest · ai · ai-worker · postgres · redis
                                                                  (Docker network only; nothing else is published)

Settings and secrets: SSM Parameter Store ─► /opt/readmely/.env on the instance (root only)
Backups:              pg_dump every night ─► S3 (encrypted, kept 14 days)
Deploys:              GitHub Actions ─OIDC─► push to ECR ─► SSM Run Command ─► scripts/deploy.sh
```

- **Only Vercel can use the API.** Browsers never talk to AWS; the web server calls the API on
  their behalf (`apps/web/src/lib/api-proxy.ts`). The web server sends a shared key
  (`API_ORIGIN_KEY`), and Caddy answers `404` to anything without it. Vercel has no fixed outbound
  addresses, so a key is used rather than an IP allowlist.
- **No SSH.** Port 22 is closed. You get a shell through SSM Session Manager, and deploys run
  through SSM Run Command.
- **No long-lived AWS keys anywhere.** The instance uses an instance role. GitHub Actions signs in
  with OIDC, and only from this repository's `production` environment.
- **Real visitor addresses.** Vercel sets `X-Forwarded-For` to the visitor's address. Caddy passes
  it on, and the API believes it only from Caddy's fixed address (`TRUSTED_PROXIES`), so rate
  limits apply per visitor.

The files involved:

| Path | What it is |
|---|---|
| `infra/prod/docker-compose.prod.yml` | The production stack, with memory limits for a 2 GB machine |
| `infra/prod/Caddyfile` | TLS, the origin-key gate and routing |
| `infra/prod/scripts/bootstrap.sh` | EC2 user data: Docker, Compose, swap, automatic security updates |
| `infra/prod/scripts/render-env.sh` | Writes `/opt/readmely/.env` from Parameter Store, with checks |
| `infra/prod/scripts/deploy.sh` | Pull, migrate, start, health check, install the backup timer |
| `infra/prod/scripts/backup.sh` | `pg_dump` to S3 |
| `infra/prod/scripts/compose.sh` | `docker compose` with the right files, for everyday operations |
| `infra/aws/*.json` | IAM policies, lifecycle rules and budgets used by the commands below |
| `.github/workflows/deploy.yml` | Build, push and deploy |

## What it costs

Prices are for us-east-1, on demand, and come out of your credits first.

| Item | Per month |
|---|---|
| EC2 t3.small (2 vCPU, 2 GB), CPU credits in `standard` mode | $15.2 |
| EBS gp3, 30 GB, encrypted | $2.4 |
| Public IPv4 address (the Elastic IP) | $3.7 |
| ECR (about 0.5 GB of images) and S3 (backups and release bundles) | about $0.1 |
| Budgets, Cost Anomaly Detection, Parameter Store (standard), IAM, SSM, CloudWatch basic metrics and up to 10 alarms | free |
| **Total** | **about $21.5** |

$100 of credits lasts about 4½ months, and $200 about 9. To stretch them, use a **t3.micro**
($7.6) instead. It has 1 GB of memory, so it leans on the 2 GB swap file. Lower the `mem_limit`
values in the compose file, and expect slow analyses of large repositories.

> **Your account is on the paid plan.** When the credits run out or expire, AWS charges your card
> without asking. The zero-spend budget in step 1 tells you about the first cent. Check the credit
> balance and **expiry date** under *Billing and Cost Management → Credits*, and put a reminder in
> your calendar a week before it.

Things that would cost money and are deliberately left out:
- RDS and ElastiCache: Postgres and Redis run on the instance.
- A NAT gateway or load balancer.
- GuardDuty after its trial.
- CloudWatch Logs: logs stay on the instance and are rotated.
- EBS snapshots: database dumps go to S3 instead.

## 0. Secure the account

Do this before creating anything.

1. **Root user.** Sign in as root, open *IAM → Security credentials*, and **assign an MFA device**
   (a passkey or an authenticator app). Delete any root access keys. After this step, use root
   only for billing emergencies.
2. **Your everyday user.** Open *IAM Identity Center → Enable*. Create a user for yourself and a
   permission set from `AdministratorAccess`, assign it to the account, and turn on MFA. Sign in
   through the access portal from now on.
3. **CLI.** Install the AWS CLI v2, run `aws configure sso` with the portal URL, then
   `export AWS_PROFILE=<the profile name>`. Check with `aws sts get-caller-identity`.
4. **Account-wide guards** (they cost nothing):

```sh
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export AWS_REGION=us-east-1   # match your Vercel project's function region (default iad1 = us-east-1)

# No S3 bucket in this account can ever be made public.
aws s3control put-public-access-block --account-id "$ACCOUNT_ID" --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true

# Every new EBS volume is encrypted.
aws ec2 enable-ebs-encryption-by-default

# Flags anything shared outside the account (an S3 bucket, a role another account can assume).
aws accessanalyzer create-analyzer --analyzer-name account --type ACCOUNT
```

## 1. Billing alerts

Set these up before launching anything. Replace `you@example.com` with your email address.
Every budget emails you a confirmation link, so confirm each one.

```sh
EMAIL=you@example.com
cd infra/aws

# a) Zero spend: emails you as soon as anything is charged AFTER credits (credits included in the sum).
sed "s|<EMAIL>|$EMAIL|" budget-zero-spend-alerts.json > /tmp/zero-alerts.json
aws budgets create-budget --account-id "$ACCOUNT_ID" \
  --budget file://budget-zero-spend.json --notifications-with-subscribers file:///tmp/zero-alerts.json

# b) Monthly usage of $25 with credits EXCLUDED: tracks how fast the credits are going.
#    Emails at 50%, 80% and 100% of actual usage, and when the forecast passes 100%.
sed "s|<EMAIL>|$EMAIL|" budget-monthly-alerts.json > /tmp/monthly-alerts.json
aws budgets create-budget --account-id "$ACCOUNT_ID" \
  --budget file://budget-monthly.json --notifications-with-subscribers file:///tmp/monthly-alerts.json

# c) Cost anomaly detection: a daily email when any service costs $5 more than usual.
#    New accounts often have a default monitor already: `aws ce get-anomaly-monitors`. Reuse its ARN if so.
MONITOR_ARN=$(aws ce create-anomaly-monitor --anomaly-monitor \
  '{"MonitorName":"services","MonitorType":"DIMENSIONAL","MonitorDimension":"SERVICE"}' \
  --query MonitorArn --output text)
aws ce create-anomaly-subscription --anomaly-subscription "{
  \"SubscriptionName\": \"daily-anomalies\", \"Frequency\": \"DAILY\", \"MonitorArnList\": [\"$MONITOR_ARN\"],
  \"Subscribers\": [{\"Type\": \"EMAIL\", \"Address\": \"$EMAIL\"}],
  \"ThresholdExpression\": {\"Dimensions\": {\"Key\": \"ANOMALY_TOTAL_IMPACT_ABSOLUTE\",
    \"MatchOptions\": [\"GREATER_THAN_OR_EQUAL\"], \"Values\": [\"5\"]}}}"
cd ../..
```

d) **In the console**, open *Billing and Cost Management → Billing preferences → Alert
preferences* and turn on **AWS Free Tier alerts** and **CloudWatch billing alerts**.

e) **A second net: a CloudWatch alarm on the estimated bill.** This metric exists only in
us-east-1, and only after (d) has been on for a few hours.

```sh
TOPIC_ARN=$(aws sns create-topic --region us-east-1 --name billing-alerts --query TopicArn --output text)
aws sns subscribe --region us-east-1 --topic-arn "$TOPIC_ARN" --protocol email --notification-endpoint "$EMAIL"
aws cloudwatch put-metric-alarm --region us-east-1 --alarm-name estimated-charges-over-30-usd \
  --namespace AWS/Billing --metric-name EstimatedCharges --dimensions Name=Currency,Value=USD \
  --statistic Maximum --period 21600 --evaluation-periods 1 --threshold 30 \
  --comparison-operator GreaterThanThreshold --alarm-actions "$TOPIC_ARN"
```

Check: *Budgets* lists both budgets as **OK**. You received (and clicked) the SNS and budget
confirmation emails.

## 2. Shell variables for the rest of this guide

```sh
export AWS_REGION=us-east-1
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
OPS_BUCKET=readmely-ops-$ACCOUNT_ID                  # bucket names are global; the account id makes it unique
GITHUB_REPO=Gitax18/readmely
WEB_ORIGIN=https://readmely.gitanshu.me              # the web app's address on Vercel, no trailing slash
fill() {  # fill in the placeholders of a JSON template in infra/aws
  sed -e "s|<ACCOUNT_ID>|$ACCOUNT_ID|g" -e "s|<REGION>|$AWS_REGION|g" \
      -e "s|<OPS_BUCKET>|$OPS_BUCKET|g" -e "s|<GITHUB_REPO>|$GITHUB_REPO|g" "infra/aws/$1"
}
```

Run all commands from the repository root.

## 3. Storage: ECR repositories and the ops bucket

```sh
# One repository per image. Scanned on push, and only the last 5 images are kept (rollback targets).
for repo in api ingest migrate ai; do
  aws ecr create-repository --repository-name "readmely/$repo" \
    --image-scanning-configuration scanOnPush=true --image-tag-mutability IMMUTABLE \
    --encryption-configuration encryptionType=AES256
  aws ecr put-lifecycle-policy --repository-name "readmely/$repo" \
    --lifecycle-policy-text file://infra/aws/ecr-lifecycle.json
done

# The ops bucket holds release bundles (releases/) and database dumps (backups/).
# It is private and encrypted; old objects expire (backups after 14 days, releases after 30).
aws s3api create-bucket --bucket "$OPS_BUCKET"   # outside us-east-1 add: --create-bucket-configuration LocationConstraint=$AWS_REGION
aws s3api put-public-access-block --bucket "$OPS_BUCKET" --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
aws s3api put-bucket-encryption --bucket "$OPS_BUCKET" --server-side-encryption-configuration \
  '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
aws s3api put-bucket-lifecycle-configuration --bucket "$OPS_BUCKET" \
  --lifecycle-configuration file://infra/aws/ops-bucket-lifecycle.json
```

## 4. Network: security group and Elastic IP

Use the default VPC. Port **443** serves the API. Port **80** is used only for the certificate
challenge and the redirect to HTTPS. **No port 22.**

```sh
VPC_ID=$(aws ec2 describe-vpcs --filters Name=is-default,Values=true --query 'Vpcs[0].VpcId' --output text)
SG_ID=$(aws ec2 create-security-group --group-name readmely-api --vpc-id "$VPC_ID" \
  --description "Readmely API: HTTPS and the ACME challenge only" --query GroupId --output text)
aws ec2 authorize-security-group-ingress --group-id "$SG_ID" --ip-permissions \
  'IpProtocol=tcp,FromPort=443,ToPort=443,IpRanges=[{CidrIp=0.0.0.0/0,Description=HTTPS}]' \
  'IpProtocol=tcp,FromPort=80,ToPort=80,IpRanges=[{CidrIp=0.0.0.0/0,Description=ACME and redirect}]'

ALLOC_ID=$(aws ec2 allocate-address --domain vpc \
  --tag-specifications 'ResourceType=elastic-ip,Tags=[{Key=app,Value=readmely}]' --query AllocationId --output text)
EIP=$(aws ec2 describe-addresses --allocation-ids "$ALLOC_ID" --query 'Addresses[0].PublicIp' --output text)

# The API's free hostname: sslip.io resolves api.<a-b-c-d>.sslip.io to a.b.c.d, and Caddy gets a real
# certificate for it. With your own domain, add an A record (e.g. api.readmely.gitanshu.me -> $EIP) and use that.
API_HOST="api.${EIP//./-}.sslip.io"
echo "$API_HOST"
```

## 5. IAM roles

**The instance role** can read its settings, pull images, fetch release bundles, write backups,
and be managed through SSM. Nothing else.

```sh
aws iam create-role --role-name readmely-ec2 --assume-role-policy-document file://infra/aws/ec2-trust.json
aws iam attach-role-policy --role-name readmely-ec2 \
  --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore
aws iam attach-role-policy --role-name readmely-ec2 \
  --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly
aws iam put-role-policy --role-name readmely-ec2 --policy-name readmely \
  --policy-document "$(fill ec2-instance-policy.json)"
aws iam create-instance-profile --instance-profile-name readmely-ec2
aws iam add-role-to-instance-profile --instance-profile-name readmely-ec2 --role-name readmely-ec2
```

**The deploy role** is assumed by GitHub Actions through OIDC. It can push the four images, upload
a release bundle, and run a shell command on instances tagged `app=readmely`.

```sh
aws iam create-open-id-connect-provider --url https://token.actions.githubusercontent.com \
  --client-id-list sts.amazonaws.com
aws iam create-role --role-name readmely-deploy --assume-role-policy-document "$(fill deploy-trust.json)"
aws iam put-role-policy --role-name readmely-deploy --policy-name deploy \
  --policy-document "$(fill deploy-policy.json)"
DEPLOY_ROLE_ARN=$(aws iam get-role --role-name readmely-deploy --query Role.Arn --output text)
```

> The trust policy accepts only jobs that run in the repository's **`production` environment**.
> If GitHub does not offer environments for your private repository on your plan, trust tags
> instead. In `deploy-trust.json`, change `StringEquals` on `:sub` to
> `"StringLike": {"token.actions.githubusercontent.com:sub": "repo:<GITHUB_REPO>:ref:refs/tags/v*"}`,
> remove `environment: production` from the workflow, and set the variables in step 8 as
> repository variables.

## 6. Secrets and settings

Every setting lives in Parameter Store under `/readmely/prod/`. Secrets are `SecureString`,
encrypted with the free AWS-managed key. `render-env.sh` turns them into `/opt/readmely/.env`
on every deploy, so **to change a setting, change the parameter and deploy again.**

```sh
put()    { aws ssm put-parameter --name "/readmely/prod/$1" --type String       --value "$2" --overwrite >/dev/null; }
secret() { aws ssm put-parameter --name "/readmely/prod/$1" --type SecureString --value "$2" --overwrite >/dev/null; }

# Plain settings
put API_HOST     "$API_HOST"
put WEB_ORIGIN   "$WEB_ORIGIN"
put OPS_BUCKET   "$OPS_BUCKET"
put POSTGRES_USER readmely
put POSTGRES_DB   readmely

# Generated secrets: letters and digits only, as .env.example asks. Run this ONCE; running it again
# changes the database passwords (fine, deploy sets them) and AUTH_CODE_KEY (ends pending sign-ins).
for name in POSTGRES_PASSWORD APP_DB_PASSWORD AI_DB_PASSWORD REDIS_PASSWORD; do
  secret "$name" "$(openssl rand -hex 24)"
done
for name in AUTH_CODE_KEY AI_SERVICE_TOKEN ORIGIN_KEY; do
  secret "$name" "$(openssl rand -hex 32)"
done
```

Then add the third-party settings. Production refuses to start without a real mailer.

```sh
# Email (Brevo): see .env.example for where each value comes from.
put    SMTP_HOST      smtp-relay.brevo.com
put    SMTP_PORT      587
put    MAIL_FROM      hello@readmely.gitanshu.me      # a sender Brevo has verified
put    MAIL_FROM_NAME Readmely
secret SMTP_USERNAME  '<brevo smtp login>'
secret SMTP_PASSWORD  '<brevo smtp key>'

# Optional: each pair is all or nothing; a provider without credentials shows no button.
secret GOOGLE_CLIENT_ID     '<id>'
secret GOOGLE_CLIENT_SECRET '<secret>'
secret GITHUB_CLIENT_ID     '<id>'
secret GITHUB_CLIENT_SECRET '<secret>'
secret GITHUB_API_TOKEN     '<read-only token, no scopes>'
secret GITHUB_TOKEN_KEY     "$(openssl rand -hex 32)"   # private repositories; never change it afterwards
secret OPENAI_API_KEY       '<key>'
put    LLM_DAILY_CAP_USD    3                          # hard daily cap on model spend
```

Values must fit on one line and must not contain a single quote. `render-env.sh` checks this, and
checks that every required setting is present, before anything starts.

## 7. Launch the instance

```sh
INSTANCE_ID=$(aws ec2 run-instances \
  --image-id resolve:ssm:/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
  --instance-type t3.small \
  --credit-specification CpuCredits=standard \
  --iam-instance-profile Name=readmely-ec2 \
  --security-group-ids "$SG_ID" \
  --metadata-options HttpTokens=required,HttpPutResponseHopLimit=1,HttpEndpoint=enabled \
  --block-device-mappings '[{"DeviceName":"/dev/xvda","Ebs":{"VolumeSize":30,"VolumeType":"gp3","Encrypted":true,"DeleteOnTermination":true}}]' \
  --user-data file://infra/prod/scripts/bootstrap.sh \
  --disable-api-termination \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=readmely},{Key=app,Value=readmely}]' \
                       'ResourceType=volume,Tags=[{Key=app,Value=readmely}]' \
  --query 'Instances[0].InstanceId' --output text)

aws ec2 wait instance-status-ok --instance-ids "$INSTANCE_ID"
aws ec2 associate-address --instance-id "$INSTANCE_ID" --allocation-id "$ALLOC_ID"
```

Why these flags:

| Flag | Reason |
|---|---|
| `CpuCredits=standard` | A t3 in the default `unlimited` mode bills extra for sustained CPU use. `standard` throttles instead, so there are no surprise charges. |
| `HttpTokens=required`, hop limit 1 | IMDSv2 only, and containers cannot reach the instance's credentials (they do not need them). |
| `--disable-api-termination` | The instance cannot be deleted by accident. |
| 30 GB gp3, encrypted | The free-tier-sized disk. |

Check that it is ready (bootstrap takes 2–3 minutes after boot):

```sh
aws ssm start-session --target "$INSTANCE_ID"        # needs the Session Manager plugin for the CLI
# on the instance:
sudo docker compose version && swapon --show && sudo tail -n 5 /var/log/cloud-init-output.log
```

## 8. First deploy

Tell GitHub where to deploy. None of these values is a secret.

```sh
gh api -X PUT "repos/$GITHUB_REPO/environments/production" >/dev/null
for kv in "AWS_REGION=$AWS_REGION" "AWS_DEPLOY_ROLE_ARN=$DEPLOY_ROLE_ARN" \
          "INSTANCE_ID=$INSTANCE_ID" "OPS_BUCKET=$OPS_BUCKET"; do
  gh variable set "${kv%%=*}" --env production --body "${kv#*=}"
done
```

Optional: under *Settings → Environments → production*, add yourself as a required reviewer, so
every deploy waits for your approval.

Deploy:

```sh
gh workflow run deploy.yml -f ref=main
gh run watch "$(gh run list --workflow deploy.yml --limit 1 --json databaseId --jq '.[0].databaseId')"
```

The workflow:
1. Builds the four images and pushes them to ECR, tagged with the commit SHA.
2. Uploads `infra/prod/` as `releases/<sha>.tgz`.
3. Runs `scripts/deploy.sh` on the instance. That script writes `.env`, pulls the images, starts
   Postgres and Redis, applies migrations (`migrate up`, then `migrate roles`), starts the rest,
   waits for health checks, checks `https://$API_HOST/healthz` (the first run also gets the
   certificate), and installs the nightly backup timer.

From then on, pushing a tag `v*` deploys automatically. You can also run the workflow by hand.

Check from your laptop:

```sh
curl -fsS "https://$API_HOST/healthz"                                    # 200: the stack is up
curl -s -o /dev/null -w '%{http_code}\n' "https://$API_HOST/v1/auth/providers"         # 404: no key
ORIGIN_KEY=$(aws ssm get-parameter --name /readmely/prod/ORIGIN_KEY --with-decryption --query Parameter.Value --output text)
curl -s -o /dev/null -w '%{http_code}\n' -H "X-Readmely-Origin-Key: $ORIGIN_KEY" "https://$API_HOST/v1/auth/providers"  # 200
curl -s -o /dev/null -w '%{http_code}\n' "https://$API_HOST/_internal/api/v1/status"   # 404: internal is keyed too
```

## 9. Point Vercel at the API

In the Vercel project, open *Settings → Environment Variables*, set these for **Production**, then
redeploy:

| Variable | Value |
|---|---|
| `API_PUBLIC_URL` | `https://<API_HOST>` |
| `API_INTERNAL_URL` | `https://<API_HOST>/_internal` |
| `API_ORIGIN_KEY` | the `ORIGIN_KEY` value from step 8 (mark it **Sensitive**) |
| `WEB_BEHIND_PROXY` | `true` (Vercel sets `X-Forwarded-For` itself) |
| `STATUS_DETAIL` | `coarse` |

Also open *Settings → Functions → Function Region* and make sure it is the same region as the
instance (`iad1` for us-east-1). Every page makes a few API calls, so a cross-ocean hop shows.

## 10. Third-party services

- **Brevo:** add the Elastic IP under *Security → Authorised IPs*. Otherwise sign-in emails fail
  with `525`.
- **Google:** in the OAuth client, set the authorised redirect URI to
  `https://readmely.gitanshu.me/api/v1/auth/google/callback`.
- **GitHub:** in the OAuth App, set the callback URL to
  `https://readmely.gitanshu.me/api/v1/auth/github/callback`.

The redirect URI is always `WEB_ORIGIN` plus that path.

Then test end to end on the live site:
- the status page shows the system as operational,
- an email code arrives,
- Google and GitHub sign-in work,
- a repository analysis runs from start to finish.

## 11. Monitoring

These use free CloudWatch basic metrics and stay within the 10 free alarms. Replace the region if
you are not in us-east-1.

```sh
OPS_TOPIC=$(aws sns create-topic --name readmely-ops --query TopicArn --output text)
aws sns subscribe --topic-arn "$OPS_TOPIC" --protocol email --notification-endpoint "$EMAIL"

# The hardware under the instance fails: move it to healthy hardware automatically (same IP, same disk).
aws cloudwatch put-metric-alarm --alarm-name readmely-system-check \
  --namespace AWS/EC2 --metric-name StatusCheckFailed_System --dimensions Name=InstanceId,Value="$INSTANCE_ID" \
  --statistic Maximum --period 60 --evaluation-periods 2 --threshold 0 --comparison-operator GreaterThanThreshold \
  --alarm-actions "arn:aws:automate:${AWS_REGION}:ec2:recover" "$OPS_TOPIC"

# The instance itself is stuck (out of memory, kernel trouble): email.
aws cloudwatch put-metric-alarm --alarm-name readmely-instance-check \
  --namespace AWS/EC2 --metric-name StatusCheckFailed_Instance --dimensions Name=InstanceId,Value="$INSTANCE_ID" \
  --statistic Maximum --period 300 --evaluation-periods 2 --threshold 0 --comparison-operator GreaterThanThreshold \
  --alarm-actions "$OPS_TOPIC"

# CPU credits running low: in standard mode the instance slows down to its baseline when they run out.
aws cloudwatch put-metric-alarm --alarm-name readmely-cpu-credits-low \
  --namespace AWS/EC2 --metric-name CPUCreditBalance --dimensions Name=InstanceId,Value="$INSTANCE_ID" \
  --statistic Minimum --period 300 --evaluation-periods 3 --threshold 30 --comparison-operator LessThanThreshold \
  --alarm-actions "$OPS_TOPIC"
```

For an outside view, a free uptime monitor (for example UptimeRobot) can poll
`https://<API_HOST>/healthz` every 5 minutes.

## Operations

Open a shell on the instance with `aws ssm start-session --target "$INSTANCE_ID"`, then `sudo -i`.
In the commands below, `c` is short for `/opt/readmely/release/scripts/compose.sh`.

| Task | How |
|---|---|
| See what is running, and memory use | `c ps` · `docker stats --no-stream` |
| Follow logs | `c logs -f --tail 100 api` (or `ingest`, `ai-worker`, `caddy`, …) |
| Deploy | `gh workflow run deploy.yml -f ref=<branch, tag or sha>`, or push a `v*` tag |
| Roll back fast | On the instance: `/opt/readmely/release/scripts/deploy.sh <registry> <older sha>`. ECR keeps the last 5 images. If the compose file also changed, restore the previous bundle first: `mv /opt/readmely/release.prev /opt/readmely/release`. |
| Change a setting or rotate a secret | Update the parameter (step 6), then deploy again. Rotating `ORIGIN_KEY` also needs Vercel's `API_ORIGIN_KEY` updated, with a redeploy on both sides. |
| Back up now | `systemctl start readmely-backup.service`, then `journalctl -u readmely-backup -n 20` |
| Restore a backup | See below |
| Restart one service | `c restart api` |

**Restore a backup** (this replaces the current data; take a backup first):

```sh
source <(grep -E '^(OPS_BUCKET|POSTGRES_USER|POSTGRES_DB)=' /opt/readmely/.env)
aws s3 ls "s3://$OPS_BUCKET/backups/"
c stop api ingest ai ai-worker
aws s3 cp "s3://$OPS_BUCKET/backups/<date>.dump" - \
  | c exec -T postgres pg_restore --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --clean --if-exists --no-owner
c up -d
```

Test a restore once, into a scratch database, so you know it works before you need it.

## Security checklist

- [ ] Root user has MFA and no access keys. Daily work uses an Identity Center user with MFA.
- [ ] Account-wide S3 Block Public Access and EBS encryption by default are on. Access Analyzer reports nothing unexpected.
- [ ] The security group allows only 80 and 443. Nothing allows 22.
- [ ] IMDSv2 is required, with hop limit 1.
- [ ] The instance role and deploy role have only the policies in `infra/aws/`. There are no IAM users with access keys.
- [ ] Every secret is a `SecureString`. `/opt/readmely/.env` is mode `600`, owned by root. Nothing secret is in git (CI runs gitleaks).
- [ ] Without the origin key, `https://<API_HOST>/` and `/_internal/*` return `404`.
- [ ] Postgres and Redis have no published ports (`c ps` shows ports only for Caddy).
- [ ] Containers run read-only, with all capabilities dropped and `no-new-privileges`, as in development. ECR scans every image on push.
- [ ] Security updates install daily (`systemctl status dnf-automatic.timer`).
- [ ] Backups land in S3 nightly, and a restore has been tested.
- [ ] `LLM_DAILY_CAP_USD` is set, and the OpenAI project has its own usage limit.

## Tearing it all down

Everything that bills, in the order to remove it:

```sh
aws ec2 modify-instance-attribute --instance-id "$INSTANCE_ID" --no-disable-api-termination
aws ec2 terminate-instances --instance-ids "$INSTANCE_ID"          # also deletes its disk
aws ec2 release-address --allocation-id "$ALLOC_ID"                # an unattached IP still costs money
for repo in api ingest migrate ai; do aws ecr delete-repository --repository-name "readmely/$repo" --force; done
aws s3 rb "s3://$OPS_BUCKET" --force
```

Then delete the security group, the two roles and the instance profile, the OIDC provider, the
`/readmely/prod/` parameters and the alarms. All of these are free, but there is no reason to keep
them. Keep the budgets until the account is closed.
