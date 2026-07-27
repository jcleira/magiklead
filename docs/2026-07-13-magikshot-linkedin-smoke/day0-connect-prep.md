# Day-0 connect-ceremony prep — evidence (issue #06)

Executed 2026-07-27 against the smoke pod (`smoke.magiklead.localhost`)
and the public tunnel `https://magiklead-smoke.magikshot.com`. No key
material here — the `Unipile-Auth` header value is
`UNIPILE_WEBHOOK_SECRET`, masked in every capture below.

This is the **session-driven prep** of #06 (step 1 + ACs 2–3). The
founder ceremony (create the dedicated LinkedIn account, complete
hosted-auth) is **not** done — `linkedin_accounts` is still empty. See
the Handoff in `issues/06-day-one-connect-ceremony.md`.

## Deploy gap closed first

Slices #02/#03/#07 were written + tested in the mvp worktree but left
uncommitted, so the smoke pod (a separate worktree) was running
pre-#02 code: `APP_URL` was the local `.localhost` literal and the
webhook CLI was absent. Closed by committing the work (PR #7),
fast-forwarding `magiklead-smoke`, and `devpods up` with
`MAGIKLEAD_PUBLIC_API_URL` exported.

Verified after:

| Check | Result |
|-------|--------|
| `devpods exec api printenv APP_URL` | `https://magiklead-smoke.magikshot.com` |
| `schema_migrations` | version 30, `dirty=f` (migration 030 applied) |
| tunnel `GET /api/v1/webhooks/unipile` | HTTP 405 (reaches Traefik → api-smoke) |
| founder onboarded | Clerk user `user_3DIGNMGwSJFoQg7l40AvZO71HXp` (`jmc.leira@gmail.com`) present |
| `linkedin_accounts` | 0 rows (ceremony not done) |

## AC2 — webhook registration at the tunnel

`register --base-url https://magiklead-smoke.magikshot.com --name-prefix magiklead-smoke-`,
then `list`:

```
3 registration(s)
  0xdcIi3wQ7y3b7aStm_xfA  messaging       magiklead-smoke-messaging       https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile  message_received  true
  8sSiYpgGRW-gBH5BmVMO_g  users           magiklead-smoke-users           https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile  new_relation      true
  Koycuj6RSXyd-8QK1QYeTw  account_status  magiklead-smoke-account_status  https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile  -                 true
```

- `Unipile-Auth` header present on all three (`GET /api/v1/webhooks`,
  value masked).
- Idempotent re-run reports `created=1 kept=2`.
- 3 stale `magiklead-mvp-*` registrations (pointing at a dead
  quick-tunnel `arms-gorgeous-disks-jimmy.trycloudflare.com`) pruned;
  `foreign-untouched` confirmed no other tenant's registration was
  touched.

### Bug found + fixed on the first live run (issue #03)

#03's CLI had only ever run against an httptest stub. Its first live
create against real Unipile failed: the create response is
`{"object":"WebhookCreated","webhook_id":"…"}`, but `CreateWebhook`
parsed `id` → errored "response had no id" *after* Unipile had already
created the webhook (201). The stub had encoded a fictional `{"id":…}`
body, so the tests passed while the code was wrong. Fixed in `6d59b77`
(parse `webhook_id`, rebuild the `Webhook` from the sent spec; stub
tests corrected to the real shape). `account_status` + `users` were
created during the debugging (CLI, then a diagnostic curl); the fixed
CLI created `messaging` and re-runs clean.

## AC3 — hosted-auth notify_url

`UnipileHandler.AuthURL` builds `notify_url = APP_URL +
/api/v1/webhooks/unipile`. `APP_URL` is now the tunnel (above), so a
minted link carries
`https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile`. The
live outbound request is captured when the founder clicks **Connect
LinkedIn** during the ceremony (api log) — that closes AC3 alongside
AC4. Config is verified; no link has been minted yet.
