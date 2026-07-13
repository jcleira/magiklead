#!/usr/bin/env bash
set -euo pipefail

# Provision the named cloudflared tunnel that fronts the magiklead
# smoke devpod API, end to end: create tunnel, route DNS, render
# config, install + enable the systemd user service, verify from
# the public internet.
#
# One-time prerequisite (browser auth, cannot be scripted):
#   cloudflared tunnel login      # pick the magikshot.com zone
#
# Usage: ./provision-tunnel.sh <public-hostname> [pod-name]
#   e.g. ./provision-tunnel.sh magiklead-smoke.magikshot.com smoke

PUBLIC_HOSTNAME=${1:?"Usage: ./provision-tunnel.sh <public-hostname> [pod-name]"}
POD=${2:-smoke}
TUNNEL_NAME=${TUNNEL_NAME:-magiklead-${POD}}
ORIGIN_HOST_HEADER="api-${POD}.magiklead.localhost"

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CF_DIR="$HOME/.cloudflared"
CONFIG="$CF_DIR/${TUNNEL_NAME}.yml"
UNIT_DIR="$HOME/.config/systemd/user"
UNIT=magiklead-smoke-tunnel.service

if [[ ! -f "$CF_DIR/cert.pem" ]]; then
  echo "ERROR: $CF_DIR/cert.pem not found." >&2
  echo "Run 'cloudflared tunnel login' first (browser auth, pick the" >&2
  echo "magikshot.com zone), then re-run this script." >&2
  exit 1
fi

echo "==> Ensuring tunnel '${TUNNEL_NAME}' exists"
if ! cloudflared tunnel list --output json | python3 -c "
import json,sys
sys.exit(0 if any(t['name']=='${TUNNEL_NAME}' for t in json.load(sys.stdin)) else 1)
"; then
  cloudflared tunnel create "$TUNNEL_NAME"
fi

TUNNEL_UUID=$(cloudflared tunnel list --output json | python3 -c "
import json,sys
print(next(t['id'] for t in json.load(sys.stdin) if t['name']=='${TUNNEL_NAME}'))
")
CREDENTIALS_FILE="$CF_DIR/${TUNNEL_UUID}.json"
[[ -f "$CREDENTIALS_FILE" ]] \
  || { echo "ERROR: credentials file $CREDENTIALS_FILE missing" >&2; exit 1; }

echo "==> Routing DNS ${PUBLIC_HOSTNAME} -> tunnel ${TUNNEL_UUID}"
cloudflared tunnel route dns --overwrite-dns "$TUNNEL_NAME" "$PUBLIC_HOSTNAME"

echo "==> Rendering ${CONFIG}"
sed -e "s|__TUNNEL_UUID__|${TUNNEL_UUID}|" \
    -e "s|__CREDENTIALS_FILE__|${CREDENTIALS_FILE}|" \
    -e "s|__PUBLIC_HOSTNAME__|${PUBLIC_HOSTNAME}|" \
    -e "s|__ORIGIN_HOST_HEADER__|${ORIGIN_HOST_HEADER}|" \
    "$HERE/cloudflared-config.template.yml" > "$CONFIG"
if [[ "$TUNNEL_NAME" != "magiklead-smoke" ]]; then
  echo "WARN: ${UNIT} points at magiklead-smoke.yml; adjust its --config for ${CONFIG}"
fi

echo "==> Installing + enabling ${UNIT}"
mkdir -p "$UNIT_DIR"
install -m 0644 "$HERE/systemd/${UNIT}" "$UNIT_DIR/"
systemctl --user daemon-reload
systemctl --user enable --now "$UNIT"
systemctl --user restart "$UNIT"

echo "==> Enabling linger so the tunnel survives reboots unattended"
loginctl enable-linger "$USER" \
  || echo "WARN: enable-linger failed — run: sudo loginctl enable-linger $USER"

echo "==> Verifying public routing (api-generated status expected)"
URL="https://${PUBLIC_HOSTNAME}/api/v1/webhooks/unipile"
for i in $(seq 1 18); do
  CODE=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$URL" || echo 000)
  case "$CODE" in
    200|401|403|405|415|422|503)
      echo "==> OK: GET ${URL} -> ${CODE} (api-generated; routing proven)"
      exit 0 ;;
    404)
      echo "WARN: 404 — tunnel up but ingress hostname mismatch?" ;;
    *)
      echo "    attempt ${i}: ${CODE} (edge/DNS still propagating)" ;;
  esac
  sleep 5
done
echo "ERROR: ${URL} never returned an api-generated status" >&2
exit 1
