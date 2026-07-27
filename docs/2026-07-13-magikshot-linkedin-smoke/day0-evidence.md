# Day-0 evidence — Unipile hygiene (issue #04)

Captured 2026-07-24 against `UNIPILE_DSN=https://api55.unipile.com:18524`,
before the disconnect. Identifiers truncated; no key material here — keys
are identified only by the first 12 hex chars of their sha256.

> **Rotation waived 2026-07-25.** The sections below were written for the
> rotate-the-key path. The founder then decided to keep the existing key
> ("it's fine, use the existing key"), so no rotation happened, the
> old/new-key checks are moot, and the pre-rotation key stash was
> shredded (it was a redundant copy of a still-live secret). What remains
> live is the clean-slate + config check, `../verify-slate.sh`.

Re-run the retained checks with:

```
docs/2026-07-13-magikshot-linkedin-smoke/verify-slate.sh
```

## Key fingerprints

| Secret | len | sha256[0:12] |
|--------|-----|--------------|
| `UNIPILE_API_KEY` (retained — rotation waived) | 53 | `dcf1e1480882` |
| `UNIPILE_WEBHOOK_SECRET` (not rotated — see Out of scope) | 41 | `8bc350533d1f` |

A day-0 key stash (`~/.config/devpods/magiklead/unipile-prerotation-key.secret`)
existed briefly to keep an "old key is dead" check runnable across a
rotation. With rotation waived there is no old key, so the stash was
shredded on 2026-07-25.

## Connected account slate — NOT clean

`GET /api/v1/accounts` with the day-0 key → **HTTP 200**, `total_count: 1`.

```json
{
  "object": "AccountList",
  "items": [
    {
      "object": "Account",
      "type": "LINKEDIN",
      "id": "-QdQs03E…",
      "name": "Jose Corral",
      "created_at": "2026-07-09T12:06:08.204Z",
      "sources": [{ "id": "-QdQs03E…_MESSAGING", "status": "OK" }],
      "connection_params": { "im": {
        "id": "ACoAAAcd…",
        "publicIdentifier": "jcleira",
        "username": "Jose Corral",
        "connection_method": "credentials",
        "organizations": [{ "name": "Tu Primera App", "messaging_enabled": true }],
        "proxy": { "country": "ES" }
      }}
    }
  ],
  "cursor": null,
  "total_count": 1,
  "status_counts": { "OK": 1, "CONNECTING": 0, "CREDENTIALS": 0, "STOPPED": 0 }
}
```

**This contradicts the issue's premise.** #04 was written expecting the
founder's personal account to have stayed gone after the June spike
cleanup. It is present, it is the founder's personal profile
(`publicIdentifier: jcleira`), its status is `OK`, and it was created
**2026-07-09T12:06:08Z** — i.e. it was reconnected *after* the cleanup,
during the devpod https/auth session of that date
(`docs/2026-07-09-devpod-https-auth-handoff.md`).

It is also live in local dev, not orphaned at the provider:

```
mvp   pod: linkedin_accounts → -QdQs03ESLmmKOITsfnuDQ  status=active
           created 2026-07-09 12:07:52+00  (104s after the Unipile connect)
smoke pod: linkedin_accounts → 0 rows
```

So the smoke pod's slate is already clean for #06; the account to remove
is one the mvp pod is actively bound to. Disconnecting it at Unipile
leaves that `active` bind row pointing at a dead provider account, and
re-obtaining it costs a full hosted-auth run with LinkedIn credentials.
Founder decision, recorded below.

## 401 shape (captured, but AC2 waived)

Both a bogus key and no key at all return the shape the June spike
documented. This was captured to back the "old key dead" check; with
rotation waived that check no longer runs, but the shape is recorded for
whoever does rotate later:

```
$ curl -H "X-API-KEY: <invalid>" "$UNIPILE_DSN/api/v1/accounts"
HTTP 401
{"status":401,"type":"errors/missing_credentials","title":"Missing credentials"}
```

## Pod state — both healthy, Unipile config live

| Pod | `GET /health` | `POST /api/v1/webhooks/unipile` with a wrong `Unipile-Auth` |
|-----|---------------|-------------------------------------------------------------|
| `api-mvp`   | 200 | 401 |
| `api-smoke` | 200 | 401 |

401 rather than 503 proves `UNIPILE_WEBHOOK_SECRET` is loaded in both api
containers — the gate is authenticating, not disabled. This is the same
assertion AC3 needs post-restart, so it is re-run by the script.

## Verifier

Before rotation was waived, `verify-rotation.sh` exited 1 with 5/3 — the
three failures being exactly the rotation/disconnect-dependent checks
(fingerprint unchanged, old key still live, `total_count 1`), proving the
harness failed for the right reasons. After the disconnect and the
waiver, the slimmed `verify-slate.sh` (key→200, `total_count 0`, both
pods healthy + gated) runs **6 passed, 0 failed** (2026-07-25).

## Resolution — personal account disconnected 2026-07-24

Founder decision: disconnect, so no path exists for the smoke to send
from the personal profile. Executed with the same call the app's own
disconnect path makes (`unipile.go:244`):

```
$ curl -X DELETE -H "X-API-KEY: …" "$UNIPILE_DSN/api/v1/accounts/-QdQs03E…"
HTTP 200
{"object":"AccountDeleted"}

$ curl -H "X-API-KEY: …" "$UNIPILE_DSN/api/v1/accounts"
HTTP 200
{"object":"AccountList","items":[],"total_count":0,
 "status_counts":{"OK":0,"CONNECTING":0,"CREDENTIALS":0,"STOPPED":0}}
```

The now-stale mvp bind row was marked rather than deleted, so the history
survives:

```
mvp pod: linkedin_accounts → -QdQs03E…  status=disconnected
         last_error='unipile account deleted 2026-07-24 (smoke #04 hygiene)'
```

Consequence to remember: mvp-pod LinkedIn dev no longer has a live
provider account. Getting it back is a fresh hosted-auth run — do it
after the smoke, not during, so the founder-identity account stays the
only connected one.

## Decisions recorded

- Rotation: **waived 2026-07-25** — founder kept the existing key
  (`sha256 dcf1e148…`). Recorded in `issues.md` next to row 4 and in
  `06-day-one-connect-ceremony.md` under `Key rotation confirmed:`.
- Personal account `jcleira` (`-QdQs03E…`): **disconnected 2026-07-24**
  (above). Unipile slate is now empty; #06 connects into a clean slate.
