#!/bin/bash
# EC2 user data for the Readmely instance (Amazon Linux 2023, x86_64). Runs once, as root, at first
# boot. It prepares the machine only; the first deploy (scripts/deploy.sh) brings the application.
# See docs/deploy/aws.md, step 6.
set -euxo pipefail

COMPOSE_VERSION=v5.6.1
COMPOSE_SHA256=47737f4236e6d3ccea7291995203f8f018058783b4c3a9469119811a0b5db8c0

dnf -y upgrade --refresh
dnf -y install docker jq dnf-automatic

# Docker Compose plugin (not packaged for AL2023), checksum pinned.
mkdir -p /usr/local/lib/docker/cli-plugins
curl -fsSL -o /usr/local/lib/docker/cli-plugins/docker-compose \
  "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-x86_64"
echo "${COMPOSE_SHA256}  /usr/local/lib/docker/cli-plugins/docker-compose" | sha256sum -c -
chmod 755 /usr/local/lib/docker/cli-plugins/docker-compose

# Container logs rotate; containers keep running while the daemon restarts for an update.
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'JSON'
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "3" },
  "live-restore": true,
  "no-new-privileges": true
}
JSON
systemctl enable --now docker

# 2 GB of swap: a safety margin for a repository analysis peak, not working memory.
if [ ! -f /swapfile ]; then
  dd if=/dev/zero of=/swapfile bs=1M count=2048
  chmod 600 /swapfile
  mkswap /swapfile
  echo '/swapfile none swap defaults 0 0' >> /etc/fstab
fi
swapon -a
echo 'vm.swappiness=10' > /etc/sysctl.d/90-readmely.conf
sysctl --system

# Security updates install themselves every day.
sed -i 's/^upgrade_type.*/upgrade_type = security/; s/^apply_updates.*/apply_updates = yes/' /etc/dnf/automatic.conf
systemctl enable --now dnf-automatic.timer

# The AWS CLI on the instance uses the instance's own region (IMDSv2).
token=$(curl -fsS -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60')
region=$(curl -fsS -H "X-aws-ec2-metadata-token: ${token}" http://169.254.169.254/latest/meta-data/placement/region)
mkdir -p /root/.aws
printf '[default]\nregion = %s\n' "${region}" > /root/.aws/config

install -d -m 700 /opt/readmely
