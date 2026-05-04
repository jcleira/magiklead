# Issues — 2026-04-30-working-mvp

Source: [prd.md](./prd.md)

| Done | # | Title | Type | Blocked by |
|------|---|-------|------|------------|
| [ ]  | 1 | [Lazy tenant bootstrap](./issues/01-lazy-tenant-bootstrap.md) | AFK | None |
| [x]  | 2 | [Devpods seed snapshot](./issues/02-devpods-seed-snapshot.md) | AFK | None |
| [x]  | 3 | [Run real ingest pipeline](./issues/03-real-ingest-run.md) | HITL | None |
| [ ]  | 4 | [Hetzner VPS + domain + api online](./issues/04-hetzner-vps-and-api.md) | HITL | None |
| [ ]  | 5 | [Frontend on Hetzner](./issues/05-frontend-on-hetzner.md) | AFK | [#4](./issues/04-hetzner-vps-and-api.md) |
| [ ]  | 6 | [Production data migration](./issues/06-production-data-migration.md) | AFK | [#3](./issues/03-real-ingest-run.md), [#4](./issues/04-hetzner-vps-and-api.md) |
| [ ]  | 7 | [Sentry error tracking](./issues/07-sentry-error-tracking.md) | HITL | [#4](./issues/04-hetzner-vps-and-api.md) |
| [ ]  | 8 | [Resend transactional email](./issues/08-resend-transactional-email.md) | HITL | [#4](./issues/04-hetzner-vps-and-api.md) |
| [ ]  | 9 | [Sender domain auth (SPF/DKIM/DMARC)](./issues/09-sender-domain-auth.md) | HITL | [#8](./issues/08-resend-transactional-email.md) |
| [ ]  | 10 | [Uptime monitoring](./issues/10-uptime-monitoring.md) | HITL | [#4](./issues/04-hetzner-vps-and-api.md) |
| [ ]  | 11 | [Postgres nightly backups](./issues/11-postgres-backups.md) | HITL | [#4](./issues/04-hetzner-vps-and-api.md), [#6](./issues/06-production-data-migration.md) |
| [ ]  | 12 | [Production smoke walkthrough](./issues/12-production-smoke-walkthrough.md) | HITL | [#1](./issues/01-lazy-tenant-bootstrap.md), [#5](./issues/05-frontend-on-hetzner.md), [#6](./issues/06-production-data-migration.md), [#7](./issues/07-sentry-error-tracking.md), [#8](./issues/08-resend-transactional-email.md), [#9](./issues/09-sender-domain-auth.md), [#10](./issues/10-uptime-monitoring.md), [#11](./issues/11-postgres-backups.md), [#13](./issues/13-email-finder-source.md) |
| [ ]  | 13 | [Email finder source (paid)](./issues/13-email-finder-source.md) | HITL | [#3](./issues/03-real-ingest-run.md) |
