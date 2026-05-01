# 11 — Postgres nightly backups

**Type**: HITL — needs Hetzner Storage Box provisioning + credentials.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#4 — Hetzner VPS + domain + api online](./04-hetzner-vps-and-api.md), [#6 — Production data migration](./06-production-data-migration.md)

## What to build

Set up nightly Postgres backups: a cron-driven `pg_dump` on the VPS writes a compressed dump to a Hetzner Storage Box (mounted via SSHFS, or copied via rsync over SSH), with a 14-day retention. After this slice, a VPS loss does not cost more than ~24 hours of data, and a verified restore procedure exists.

## Acceptance criteria

- [ ] Hetzner Storage Box provisioned (1 TB ballpark — far more than enough for 14 days of compressed dumps from an MVP-sized DB).
- [ ] Storage Box accessible from the VPS — SSHFS mount at `/mnt/backups`, or rsync over SSH with the Storage Box credentials.
- [ ] `deploy/backup.sh` script runs `pg_dump --clean --if-exists` against the production Postgres container, gzips the output, and writes to the Storage Box with a date-stamped filename (`magiklead-YYYYMMDD.sql.gz`).
- [ ] Cron entry runs `deploy/backup.sh` at 03:00 UTC daily on the VPS.
- [ ] Retention enforced: the script (or a sibling cron) prunes Storage Box files older than 14 days.
- [ ] **Restore test**: take the latest backup, restore it on a scratch Postgres instance (locally or on the VPS in a temporary container), confirm `SELECT COUNT(*) FROM persons` matches production. Document the restore command in `deploy/README.md`.
- [ ] First night's backup verified in the Storage Box the morning after deploy.

## Modules touched

- Operational + small script (`deploy/backup.sh`).

## Test prior art

- None.

## Out of scope

- MinIO / object storage backups — separate concern; defer.
- Point-in-time recovery via WAL archiving — defer until daily dumps prove insufficient.
- Off-site (geographic) replication — Hetzner Storage Box is in a Hetzner DC; if total Hetzner failure is in scope, that's a separate plan.
- Encrypted-at-rest backups — Hetzner Storage Box has its own encryption posture; if dump-level encryption is needed, that's a follow-up.
