#!/usr/bin/env bash
set -euo pipefail

# MagikLead deployment script for Hetzner VPS
# Usage: ./scripts/deploy.sh <server-ip>

SERVER=${1:?"Usage: ./scripts/deploy.sh <server-ip>"}
SSH="ssh root@$SERVER"

echo "==> Deploying MagikLead to $SERVER"

# Ensure Docker is installed
$SSH "which docker >/dev/null 2>&1 || (curl -fsSL https://get.docker.com | sh)"

# Sync code
echo "==> Syncing code..."
rsync -avz --exclude='.git' --exclude='node_modules' --exclude='.next' --exclude='tmp' --exclude='bin' \
  ./ root@$SERVER:/opt/magiklead/

# Run migrations
echo "==> Running migrations..."
$SSH "cd /opt/magiklead && docker compose -f docker-compose.prod.yml run --rm api sh -c 'cd /migrations && /api migrate'" || true

# Build and restart
echo "==> Building and restarting services..."
$SSH "cd /opt/magiklead && docker compose -f docker-compose.prod.yml up -d --build"

# Health check
echo "==> Waiting for health check..."
sleep 5
$SSH "curl -sf http://localhost:8080/health && echo ' OK' || echo ' FAILED'"

echo "==> Deploy complete"
echo "  API: https://api.magiklead.com"
echo "  Frontend: https://magiklead.com (Vercel)"
