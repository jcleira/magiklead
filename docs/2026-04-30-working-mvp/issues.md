# Issues — 2026-04-30-working-mvp

Source: [prd.md](./prd.md)

| Done | # | Title | Type | Blocked by |
|------|---|-------|------|------------|
| [ ]  | 1 | [Lazy tenant bootstrap](./issues/01-lazy-tenant-bootstrap.md) | AFK | None |
| [x]  | 2 | [Devpods seed snapshot](./issues/02-devpods-seed-snapshot.md) | AFK | None |
| [x]  | 3 | [Run real ingest pipeline](./issues/03-real-ingest-run.md) | HITL | None |
| [ ]  | 4 | [Local smoke walkthrough](./issues/04-local-smoke-walkthrough.md) | HITL | [#1](./issues/01-lazy-tenant-bootstrap.md), [#2](./issues/02-devpods-seed-snapshot.md), [#3](./issues/03-real-ingest-run.md) |
| [ ]  | 5 | [Hetzner VPS + domain + api online](./issues/05-hetzner-vps-and-api.md) | HITL | [#4](./issues/04-local-smoke-walkthrough.md) |
| [ ]  | 6 | [Frontend on Hetzner](./issues/06-frontend-on-hetzner.md) | AFK | [#5](./issues/05-hetzner-vps-and-api.md) |
| [ ]  | 7 | [Production data migration](./issues/07-production-data-migration.md) | AFK | [#3](./issues/03-real-ingest-run.md), [#5](./issues/05-hetzner-vps-and-api.md) |
| [ ]  | 8 | [Sentry error tracking](./issues/08-sentry-error-tracking.md) | HITL | [#5](./issues/05-hetzner-vps-and-api.md) |
| [ ]  | 9 | [Resend transactional email](./issues/09-resend-transactional-email.md) | HITL | [#5](./issues/05-hetzner-vps-and-api.md) |
| [ ]  | 10 | [Sender domain auth (SPF/DKIM/DMARC)](./issues/10-sender-domain-auth.md) | HITL | [#9](./issues/09-resend-transactional-email.md) |
| [ ]  | 11 | [Uptime monitoring](./issues/11-uptime-monitoring.md) | HITL | [#5](./issues/05-hetzner-vps-and-api.md) |
| [ ]  | 12 | [Postgres nightly backups](./issues/12-postgres-backups.md) | HITL | [#5](./issues/05-hetzner-vps-and-api.md), [#7](./issues/07-production-data-migration.md) |
| [ ]  | 13 | [Production smoke walkthrough](./issues/13-production-smoke-walkthrough.md) | HITL | [#4](./issues/04-local-smoke-walkthrough.md), [#6](./issues/06-frontend-on-hetzner.md), [#7](./issues/07-production-data-migration.md), [#8](./issues/08-sentry-error-tracking.md), [#9](./issues/09-resend-transactional-email.md), [#10](./issues/10-sender-domain-auth.md), [#11](./issues/11-uptime-monitoring.md), [#12](./issues/12-postgres-backups.md), [#14](./issues/14-email-finder-source.md) |
| [ ]  | 14 | [Email finder source (paid)](./issues/14-email-finder-source.md) | HITL | [#3](./issues/03-real-ingest-run.md) |
