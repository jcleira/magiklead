#!/bin/sh
# Rebuild the magiklead devpod seed archive from the current devpod's
# DB. Runs the in-tree fixture loader, then dumps the result to
# $SEEDS_DIR/db.sql.gz so subsequent `devpods up` / `devpods seed`
# invocations replay the same data without re-running the loader.
#
# Environment provided by devpods:
#   SEEDS_DIR       drop db.sql.gz + manifest.json here
#   DEVPOD_NAME     devpod name (e.g. "mvp")
#   DEVPOD_PROJECT  "magiklead"
#   DATABASE_URL    pg connection string inside the devpod
#
# Prereq: the devpod is up and migrations have applied. Run:
#   devpods up
#   devpods seed regenerate
set -eu

echo "[magiklead] running fixture loader inside the api container..."
devpods exec api go run ./cmd/seed

echo "[magiklead] dumping postgres → $SEEDS_DIR/db.sql.gz..."
mkdir -p "$SEEDS_DIR"
devpods exec postgres pg_dump -U devpod -d devpod --clean --if-exists \
    | gzip -c > "$SEEDS_DIR/db.sql.gz"

# manifest.json — devpods uses this to detect when the archive is
# stale relative to the project's migrations. Hash all up.sql files.
MIGRATION_HASH=$(cat "$(dirname "$0")/../../backend/migrations/"*.up.sql | sha256sum | cut -d' ' -f1)
cat > "$SEEDS_DIR/manifest.json" <<EOF
{
  "migration_hash": "$MIGRATION_HASH",
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo "[magiklead] seed archive ready: $SEEDS_DIR/db.sql.gz"
