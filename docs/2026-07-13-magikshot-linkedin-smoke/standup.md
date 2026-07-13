# Smoke devpod standup

State capture + re-provision guide for the dedicated smoke pod
(issue [#01](./issues/01-smoke-devpod-standup.md)). The smoke pod
holds weeks of real LinkedIn campaign state on infrastructure the
daily dev loop can never touch.

## The guardrail (read first)

**Never run `devpods seed` and never run `devpods down` against the
smoke pod** for the duration of the smoke. Both are routine on the
daily `mvp` pod and catastrophic here: seed resets the database,
down destroys the volumes. The smoke pod is only ever `devpods up`
(idempotent) and, at the very end of the smoke, decommissioned
deliberately per the runbook (issue #05). If a command you are about
to run contains `seed` or `down` and the cwd is the smoke worktree —
stop.

## What exists

| Piece | Where |
|-------|-------|
| Worktree, branch `magiklead-smoke` | `~/Code/workspaces/workspace-magiklead-smoke/magiklead` |
| Devpod `smoke` (own Postgres/Redis/MinIO) | `devpods status` in that worktree |
| Public hostname | `https://magiklead-smoke.magikshot.com` → tunnel `magiklead-smoke` |
| Tunnel service | `systemctl --user status magiklead-smoke-tunnel.service` |
| Tunnel config | `~/.cloudflared/magiklead-smoke.yml` (rendered; credentials in `~/.cloudflared/<uuid>.json`, never committed) |
| Nightly dump timer | `systemctl --user list-timers magiklead-smoke-dump.timer` (03:17, `Persistent=true`) |
| Dumps (14-day retention) | `~/.local/share/magiklead-smoke/dumps/` |
| Installed dump script | `~/.local/share/magiklead-smoke/bin/dump.sh` (copy, survives worktree removal) |

Committed sources for all of it: `devpod/smoke/`.

## Re-provision from scratch

```sh
# 1. Worktree + pod (idempotent; migrations apply on up)
git worktree add -b magiklead-smoke \
  ~/Code/workspaces/workspace-magiklead-smoke/magiklead magiklead-mvp
cd ~/Code/workspaces/workspace-magiklead-smoke/magiklead && devpods up

# 2. Nightly dumps (installs units, enables linger, runs first dump)
devpod/smoke/install-dump-timer.sh

# 3. Tunnel — one-time browser auth if ~/.cloudflared/cert.pem is
#    missing: `cloudflared tunnel login`, pick the magikshot.com zone
devpod/smoke/provision-tunnel.sh magiklead-smoke.magikshot.com smoke
```

`provision-tunnel.sh` is idempotent: creates the named tunnel only
if missing, `--overwrite-dns` re-points the hostname, re-renders the
config, re-installs + restarts the unit, then curls the public URL
until an api-generated status appears.

To restore a dump into the pod (only during deliberate recovery):

```sh
docker cp <dump> devpod-magiklead__smoke-postgres-1:/tmp/r.dump
docker exec devpod-magiklead__smoke-postgres-1 \
  pg_restore -U devpod -d devpod --clean --if-exists /tmp/r.dump
```

## How the tunnel routes

Named tunnel (stable hostname for weeks — a quick tunnel's hostname
rotates) with DNS on the magikshot.com Cloudflare zone. Ingress hits
the shared Traefik on `http://localhost:80` and presents
`originRequest.httpHostHeader: api-smoke.magiklead.localhost` — port
80 because both schemes serve and http avoids the mkcert cert-name
mismatch. Expected api-generated proof of routing on
`/api/v1/webhooks/unipile`: **405** on GET, **401** on POST with a
bad/absent signature, **503** if `UNIPILE_WEBHOOK_SECRET` is unset.

## Verification quick-reference

```sh
devpods status                                   # in the smoke worktree
curl -si https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile   # 405
devpods logs api | tail                          # shows the request line
systemctl --user status magiklead-smoke-tunnel.service
systemctl --user list-timers magiklead-smoke-dump.timer
loginctl show-user "$USER" --property=Linger     # Linger=yes
ls -lh ~/.local/share/magiklead-smoke/dumps/
```
