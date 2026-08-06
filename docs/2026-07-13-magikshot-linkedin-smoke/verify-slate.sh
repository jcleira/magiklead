#!/usr/bin/env bash
# Verifies issue #04 (Unipile hygiene) end state — the CLEAN-SLATE half.
#
#   docs/2026-07-13-magikshot-linkedin-smoke/verify-slate.sh
#
# Rotation was WAIVED on 2026-07-25 (founder: use the existing key), so
# this does not check for a rotated key or a dead old key. It certifies
# the two things #06 actually depends on:
#
#   1. the Unipile key in .env.backend works  -> GET /accounts = 200
#   2. the account slate is clean             -> total_count = 0
#   3. both pods: api /health = 200, webhook wrong-auth = 401
#      (401 not 503 proves UNIPILE_WEBHOOK_SECRET is loaded — the gate
#       is authenticating, not disabled)
#
# Never prints a key: only the sha256 fingerprint (first 12 hex chars).

set -uo pipefail

ENVF="$HOME/.config/devpods/magiklead/.env.backend"
KNOWN_KEY_SHA12="dcf1e1480882" # retained key, fingerprinted 2026-07-24
PODS=(mvp smoke)

pass=0
fail=0

ok()  { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
bad() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; fail=$((fail + 1)); }

fp() { printf %s "$1" | sha256sum | cut -c1-12; }

envval() {
  grep -E "^$1=" "$ENVF" | head -1 | cut -d= -f2- | tr -d '"'\''' | tr -d '\r\n'
}

[ -f "$ENVF" ] || { echo "missing $ENVF" >&2; exit 2; }

KEY=$(envval UNIPILE_API_KEY)
DSN=$(envval UNIPILE_DSN)
[ -n "$DSN" ] || { echo "UNIPILE_DSN unset in $ENVF" >&2; exit 2; }

KEY_FP=$(fp "$KEY")
echo "Unipile key fingerprint: $KEY_FP (day-0 was $KNOWN_KEY_SHA12; rotation waived 2026-07-25)"

# ---- AC1: key in use works -------------------------------------------
echo "AC1 — key active (accounts list 200)"
BODY=$(mktemp)
CODE=$(curl -sS -o "$BODY" -w '%{http_code}' \
  -H "X-API-KEY: $KEY" -H 'accept: application/json' \
  "$DSN/api/v1/accounts")
[ "$CODE" = "200" ] && ok "HTTP 200" || bad "HTTP $CODE — $(head -c 200 "$BODY")"

# ---- AC4: clean slate ------------------------------------------------
echo "AC4 — zero connected accounts before #06"
if [ "$CODE" = "200" ]; then
  TOTAL=$(grep -o '"total_count":[0-9]*' "$BODY" | head -1 | cut -d: -f2)
  if [ "${TOTAL:-x}" = "0" ]; then
    ok "total_count 0"
  else
    bad "total_count ${TOTAL:-unknown} — connected: $(grep -o '"publicIdentifier":"[^"]*"' "$BODY" | cut -d'"' -f4 | paste -sd, -)"
  fi
else
  printf '  \033[33mSKIP\033[0m accounts list unavailable (see AC1)\n'
fi

# ---- AC3: pods healthy, Unipile config live --------------------------
echo "AC3 — api healthy, webhook gate live on both pods"
for POD in "${PODS[@]}"; do
  H=$(curl -sk -o /dev/null -w '%{http_code}' "https://api-$POD.magiklead.localhost/health")
  [ "$H" = "200" ] && ok "api-$POD /health 200" || bad "api-$POD /health $H"
  W=$(curl -sk -o /dev/null -w '%{http_code}' -X POST \
    -H 'Content-Type: application/json' -H 'Unipile-Auth: deliberately-wrong' \
    -d '{"status":"CREATION_SUCCESS","account_id":"probe","name":"probe"}' \
    "https://api-$POD.magiklead.localhost/api/v1/webhooks/unipile")
  case "$W" in
    401) ok "api-$POD webhook wrong-auth 401 (Unipile config live)" ;;
    503) bad "api-$POD webhook 503 — UNIPILE_WEBHOOK_SECRET unset in the container" ;;
    *)   bad "api-$POD webhook wrong-auth $W (expected 401)" ;;
  esac
done

rm -f "$BODY"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
