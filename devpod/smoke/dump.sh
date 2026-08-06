#!/usr/bin/env bash
set -euo pipefail

# Nightly pg_dump of a magiklead devpod database to a dated file
# outside any git worktree, with retention. Written for the smoke
# pod (docs/2026-07-13-magikshot-linkedin-smoke) but parameterized.
#
# Usage: ./dump.sh [pod-name]
#   POD          pod name (default: smoke, or $1)
#   PROJECT      devpods project (default: magiklead)
#   DUMP_DIR     output dir (default: ~/.local/share/<project>-<pod>/dumps)
#   RETENTION_DAYS  delete dumps older than this (default: 14)

POD=${1:-${POD:-smoke}}
PROJECT=${PROJECT:-magiklead}
DUMP_DIR=${DUMP_DIR:-"$HOME/.local/share/${PROJECT}-${POD}/dumps"}
RETENTION_DAYS=${RETENTION_DAYS:-14}

COMPOSE_PROJECT="devpod-${PROJECT}__${POD}"

CONTAINER=$(docker ps \
  --filter "label=com.docker.compose.project=${COMPOSE_PROJECT}" \
  --filter "label=com.docker.compose.service=postgres" \
  --format '{{.Names}}' | head -n1)

if [[ -z "$CONTAINER" ]]; then
  echo "ERROR: no running postgres container for ${COMPOSE_PROJECT}" >&2
  exit 1
fi

mkdir -p "$DUMP_DIR"
STAMP=$(date +%Y%m%d-%H%M%S)
OUT="${DUMP_DIR}/${PROJECT}-${POD}-${STAMP}.dump"

echo "==> Dumping ${COMPOSE_PROJECT} (${CONTAINER}) to ${OUT}"

# Dump inside the container (custom format), verify it is loadable
# with pg_restore --list, then copy it out. The host has no postgres
# client tools, so verification happens where they live.
docker exec "$CONTAINER" bash -c \
  'pg_dump -Fc -U devpod devpod > /tmp/nightly.dump &&
   pg_restore --list /tmp/nightly.dump > /dev/null'
docker cp "$CONTAINER":/tmp/nightly.dump "$OUT"
docker exec "$CONTAINER" rm -f /tmp/nightly.dump

echo "==> Dump OK: $(du -h "$OUT" | cut -f1) $(basename "$OUT") (pg_restore --list verified)"

echo "==> Pruning dumps older than ${RETENTION_DAYS} days"
find "$DUMP_DIR" -maxdepth 1 -name "${PROJECT}-${POD}-*.dump" \
  -mtime +"$RETENTION_DAYS" -print -delete

echo "==> Done. $(ls -1 "$DUMP_DIR" | wc -l) dump(s) retained in ${DUMP_DIR}"
