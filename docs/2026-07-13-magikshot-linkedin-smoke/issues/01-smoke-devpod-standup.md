# 01 — Smoke devpod standup: worktree, named tunnel service, nightly dumps

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately. This is the head of the
calendar-critical path (PRD: "critical path is calendar, not code").

## What to build

A dedicated smoke devpod, publicly reachable through a stable named
cloudflared tunnel, backed up nightly — so weeks of real campaign
state live on infrastructure a routine `devpods seed` or `devpods
down` on the daily dev pod can never touch.

Three pieces:

1. **Dedicated worktree + devpod.** New git worktree on its own
   branch (devpod name derives from the branch: `magiklead-mvp` →
   `mvp`, so e.g. a `magiklead-smoke` branch → `smoke` pod at
   `https://api-smoke.magiklead.localhost`). `devpods up` gives it
   its own Postgres / Redis / MinIO; migrations apply on up. The pod
   is **never seeded and never destroyed** for the smoke's duration.

2. **Named cloudflared tunnel, supervised.** A named tunnel (not a
   quick tunnel — the hostname must be stable for weeks) with a DNS
   hostname on the **magikshot.com Cloudflare zone** (e.g.
   `magiklead-smoke.magikshot.com`) whose ingress fronts the smoke
   pod's API. Route through the shared Traefik on localhost: the
   ingress rule must present the pod's Traefik host — use
   `originRequest.httpHostHeader: api-<pod>.magiklead.localhost`
   against the local http entrypoint (port 80 avoids mkcert
   cert-name mismatch; both schemes serve, no redirect). Supervise
   with a systemd **user** service (+ linger) so it survives reboots
   unattended. One-time human step if no origin cert exists yet: the
   operator runs `cloudflared tunnel login` (suggest `! cloudflared
   tunnel login` in-session).

3. **Nightly dump.** A systemd user timer that `pg_dump`s the smoke
   pod's database (via `devpods exec` or `docker exec` into the
   pod's postgres) to a dated file **outside the worktree** (e.g.
   `~/.local/share/magiklead-smoke/dumps/`), with simple retention.

Commit everything reusable production-grade (PRD user story 32):
tunnel config template (no credentials), the dump script, the unit
files, and a short standup doc in this docs folder. `scripts/deploy.sh`
is the house style for bash (`set -euo pipefail`, usage-checked args).

## Acceptance criteria

- [x] Smoke worktree + devpod exist, distinct from `mvp`; `devpods
      status` healthy; `devpods psql` shows its own database.
- [x] From the public internet, a request to
      `https://<hostname>/api/v1/webhooks/unipile` reaches the smoke
      pod's api — the response is api-generated (405 on GET, or
      503/401 on POST without secrets/header — any of these proves
      routing) and the request line appears in `devpods logs api`.
- [x] The tunnel is a systemd user service: enabled, survives
      `systemctl --user restart <unit>`, and comes back after a
      reboot with no manual action (linger enabled).
- [x] The nightly timer has produced at least one dated dump outside
      the worktree, and the dump is verified loadable (restore into a
      scratch database or `pg_restore --list`).
- [x] Tunnel config template, dump script, and unit files are
      committed with no secrets; a standup doc explains how to
      re-provision from scratch.
- [x] The never-seed / never-destroy guardrail is written down (this
      doc + the runbook's day-0 checklist, issue #05).

## Modules touched

- New ops assets — suggested home: `scripts/smoke/` or
  `devpod/smoke/` (dump script, systemd units, cloudflared config
  template).
- `docs/2026-07-13-magikshot-linkedin-smoke/` — standup notes.
- No Go or compose changes (the base-URL override is issue #02).

## Test prior art

None — ops slice. Verification is the acceptance criteria themselves
(public curl + `devpods logs` + `systemctl --user` + a restore).
Bash style prior art: `scripts/deploy.sh`.

## Out of scope

- Making the smoke pod *advertise* the tunnel URL (`APP_URL`
  override) — issue #02.
- Registering Unipile webhooks against the hostname — issue #03
  builds the tool, issue #06 runs it live.
- Any production deployment (PRD out of scope).
