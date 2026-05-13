# 16 — Hetzner VPS + api online

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#15 — Local 3-day real test](./15-local-3-day-real-test.md)

## What to build

Provision a Hetzner Cloud VPS, register `magiklead.com` (or
operator-confirmed domain), point DNS at the VPS, install Docker
+ docker-compose, and deploy the api + Postgres + Redis + S3-
compatible object storage stack with Caddy as a reverse proxy
fronting auto-LetsEncrypt SSL.

After this slice, `https://api.magiklead.com/health` returns `ok`
over HTTPS with a valid certificate, and the production stack
survives a VPS reboot via systemd.

## Acceptance criteria

- [ ] Domain registered with the operator's chosen registrar; A
      record points at the Hetzner VPS public IP.
- [ ] Hetzner Cloud VPS provisioned (CCX23 / CPX31 ballpark —
      4 vCPU / 16 GB RAM start). Operator's SSH key authorised.
- [ ] Docker + docker-compose installed on the VPS.
- [ ] `deploy/` directory at repo root contains:
      `docker-compose.prod.yml`, `Caddyfile`,
      `.env.production.example`, and a deploy script (`deploy.sh`
      or `make deploy`).
- [ ] `docker-compose.prod.yml` runs services: `api`, `worker`,
      `postgres`, `redis`, S3-compatible object storage (MinIO
      container, or wired to Hetzner Object Storage S3 endpoint —
      operator's choice), `caddy`.
- [ ] `Caddyfile` routes `api.magiklead.com` → api container with
      auto-LetsEncrypt.
- [ ] Production env file is owned by the operator (placed at
      e.g. `/etc/magiklead/.env.production` on the VPS,
      gitignored). The committed `.env.production.example`
      template captures every variable the stack reads.
- [ ] Backend migrations run on api startup (existing
      `cmd/migrate` invocation in the entrypoint, same pattern as
      the dev devpod).
- [ ] The compose stack is wrapped in a systemd unit
      (`/etc/systemd/system/magiklead.service`) that runs
      `docker-compose ... up -d` and restarts on failure;
      `systemctl restart magiklead` recovers the box.
- [ ] `curl https://api.magiklead.com/health` returns `ok` with a
      valid LetsEncrypt cert.
- [ ] Deploy script: pulls latest from git, rebuilds containers,
      runs migrations, rolls services with healthcheck-aware
      restarts (compose `up -d --build` with healthchecks suffices
      at this scale).
- [ ] Operator-facing notes (in `deploy/README.md`): SSH key
      location, Hetzner project name, domain registrar, TTL
      chosen for DNS records.

## Modules touched

- Production deploy module (new — `deploy/` directory at repo
  root).

## Test prior art

- `devpod/compose.yml` is the structural template — same
  services, with dev-only Traefik labels swapped for prod-grade
  Caddy file routing.
- No automated tests for this slice; verification is `curl`,
  browser, and reboot-recovery test.

## Out of scope

- Frontend deploy — see [#17](./17-frontend-on-hetzner.md).
- Production database content — empty DB on first deploy;
  populated by [#18](./18-production-data-migration.md).
- Sentry / monitoring / backups — see [#19](./19-sentry-error-tracking.md),
  [#22](./22-uptime-monitoring.md), [#23](./23-postgres-backups.md).
- Production Clerk + Stripe key swap — see
  [#24](./24-production-keys-swap.md). This slice deploys with
  *dev* Clerk + Stripe keys initially; the swap is a separate
  slice to keep the deploy and the key-swap independently
  diagnosable if either goes wrong.
- Multi-VPS or HA layout — single VPS, one process per service.
