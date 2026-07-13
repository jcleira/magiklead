# Handoff — magiklead devpod: https + auth + Unipile (2026-07-09)

Written after a long, messy debugging session. This captures **exact
current state**, **every change made**, and **the one open issue** so a
fresh session (or the user) can continue without re-deriving anything.

---

## TL;DR

- The devpod app is served behind a **custom https layer** (mkcert +
  shared Traefik `websecure`). This is a user-built layer; **devpods
  itself only ever emits http URLs**.
- Multiple things broke and were fixed today (cert trust, Unipile creds,
  http/https CORS mismatch). All **verified fixed** — see "Verified
  working".
- **One open issue:** after signing in, the **dashboard renders as
  logged-out**, even though authenticated API calls succeed (`GET
  /api/v1/campaigns → 200`, no 401s). This is a **client-side Clerk
  session issue**, most likely a session established under http vs the
  https origin you're now on. **Leading fix: clear site data for
  `mvp.magiklead.localhost` in Brave and sign in fresh over https.** Not
  yet tried/verified.

---

## The one open issue (start here)

**Symptom (user):** clicking Sign In redirects to the dashboard (so
Clerk client thinks you're signed in), but the dashboard shows a
logged-out state.

**What's proven:**
- Authenticated API calls now reach the backend and succeed:
  `GET /api/v1/campaigns → 200` (repeatedly), **no 401/403**, no CORS
  block. So backend token validation is fine and CORS is fixed.
- Therefore this is **not** a backend, CORS, or network problem. It is
  **Clerk client-side session state** in the browser.

**Leading hypothesis:** the Clerk session (`__session` / `__client`
cookies for the dev instance `funky-trout-94.clerk.accounts.dev`) was
created while the origin was **http://mvp.magiklead.localhost**, but the
app is now **https://mvp.magiklead.localhost**. Browsers scope cookies /
storage per-origin-scheme, so Clerk's client can't reconcile the session
→ renders signed-out, while an in-memory/cached token still lets some API
calls 200.

**Recommended next steps (in order):**
1. In Brave, open DevTools → Application → Storage → **Clear site data**
   for `mvp.magiklead.localhost` (cookies + local/session storage), then
   go to `https://mvp.magiklead.localhost` and **sign in fresh**. Verify
   the dashboard now shows signed-in. This is the most likely fix and
   costs nothing.
2. If still broken: in DevTools → Application → Cookies, inspect the
   Clerk cookies for `mvp.magiklead.localhost` — check the `Secure` flag
   and domain. A `Secure` cookie set over https won't be sent over http
   and vice-versa. Confirm the session cookie exists and is scoped to the
   https origin.
3. If still broken: check Clerk client init. The dev key is
   `pk_test_...` (instance `funky-trout-94`). Confirm the frontend's
   Clerk provider `publishableKey` and that no `proxyUrl`/`domain` is
   mis-set (the SSR logs showed `proxyUrl=""` / `domain=""`, which is
   normal for dev). A known-relevant gotcha from the frontend logs: a
   **React hydration mismatch** on `<html className=...>` (geist font
   classes differ server vs client) — probably benign, but worth ruling
   out if the dashboard's auth gate is hydration-sensitive.
4. Clerk dashboard prereq (from repo CLAUDE.md): a JWT template named
   **`magiklead-backend`** must exist with body
   `{"email": "{{user.primary_email_address}}"}`. Backend GETs are
   returning 200 so the token is currently accepted, but if fresh sign-in
   changes behavior, re-verify this template exists.

**How to watch it live:** tail the api and observe request status while
the user interacts:
`docker logs -f devpod-magiklead__mvp-api-1 2>&1 | grep -aE 'GET|POST|401|403'`
— authed GETs should be **200**. If you see **401**, the token is being
rejected (different problem than the client-render issue).

---

## Verified working (do not re-investigate)

- **https loads with a trusted cert.** Traefik serves the mkcert cert
  (`issuer=mkcert development CA`), SANs cover `*.magiklead.localhost`,
  passes real CA validation. `https://mvp.magiklead.localhost` → 200.
- **Brave cert error was fixed by restarting Brave.** Root cause: Brave
  had been running since **Jul 6 17:03**, but the mkcert CA was
  **regenerated Jul 7 17:10** and re-added to Brave's NSS store
  (`~/.pki/nssdb`) at **Jul 7 17:47**. Chromium loads its cert store only
  at startup, so Brave held a stale CA and rejected the new cert
  (`NET::ERR_CERT_AUTHORITY_INVALID`). A full Brave restart reloads the
  store. If it recurs after another CA regeneration, restart Brave again.
- **CORS over https is fixed.** `OPTIONS /api/v1/settings` with
  `Origin: https://mvp.magiklead.localhost` →
  `access-control-allow-origin: https://mvp.magiklead.localhost`. Authed
  `GET`s return 200.
- **Unipile is live on a new paid instance.** `GET /accounts → 200`;
  account "Jose Corral" present (`id Fkiw0HOvQVqUUSuvd-HV_w`, source
  status `CREDENTIALS` = wants a re-auth). Hosted-auth link mints:
  `POST /hosted/accounts/link → 201`. So **LinkedIn "Connect" will start**
  (it was 503-ing before because the old trial instance died).

---

## Every change made today (inventory — nothing hidden)

### devpods tool repo (`/home/arvos/Code/repositories/devpods`)
- `compose/traefik.yml`: restored the https config the shared Traefik
  needs — `websecure :443` entrypoint, `--providers.file.directory=
  /etc/traefik/dynamic` + watch, and a bind-mount of
  `~/.config/devpods/traefik` (which holds `tls.yml` + the mkcert
  `devpods.pem`). **NOTE:** the user subsequently hand-edited this file
  (removed the http→https redirect; kept `websecure`+tls+file provider).
  Current running Traefik has **no redirect** (http and https both serve
  directly). Uncommitted.
- Binary rebuilt (`make build`, embeds `compose/`) so `devpods up` keeps
  the https Traefik config. `devpods up` uses `traefik.EnsureRunning`
  (starts only if not already running) so it will **not** clobber a
  running Traefik.
- **Devpods emits http URLs by design** (`internal/naming/naming.go`
  hardcodes `http://`, no https/tls knob, never had one in git history).
  This is why the app URLs kept coming back http.

### User devpod config (`~/.config/devpods/magiklead/`)
- `.env.backend`: `UNIPILE_DSN` → `https://api55.unipile.com:18524`
  (new paid instance; old `:18520` trial died → 503 `no_client_session`).
  `UNIPILE_API_KEY` → new key (user-provided). `FRONTEND_URL` → https.
- `.env.frontend`: `NEXT_PUBLIC_API_URL` → https.
- `traefik/` (pre-existing, user's): `tls.yml` + `certs/devpods.pem`
  (mkcert). The cert covers all mag-family `*.localhost`. Regenerate with
  the recipe in `certs/README` if a new project/SAN is needed.

### magiklead repo (`/home/arvos/Code/workspaces/workspace-magiklead-mvp/magiklead`)
- `devpod/compose.yml`: the 4 app URL vars switched from devpods' http
  `${API_URL}`/`${FRONTEND_URL}` to **https literals** built from
  `${DEVPOD_NAME}`/`${DEVPOD_PROJECT}`:
  - api service: `APP_URL`, `FRONTEND_URL`
  - frontend service: `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_APP_URL`
  This is the fix that made CORS match the https browser origin. **This
  is a tracked repo file — shows in `git status`; keep or revert.**
- `docs/2026-06-25-finish-unipile-integration/spike-captures.md` +
  `issues/01-validation-spike.md` + `issues.md`: updated earlier in the
  Unipile spike work (separate track; slice #1 was completed).

### Unipile dashboard (external)
- Old trial instance (`:18520`) abandoned: LinkedIn account disconnected,
  webhook registrations deleted. New paid instance (`:18524`) is live.
- Webhook registrations on the **new** instance are **not** set up yet
  (the capture tunnel churned across http/https/:443 during the session).
  Not needed for "Connect" to start, but needed to capture
  `account.connected` / message events. Re-register against a live
  cloudflared tunnel when doing the capture/bind work (recipe in
  spike-captures.md). Also note: **auto-bind is a known unimplemented gap
  (issue #3)** — connecting at Unipile won't auto-reflect in the app yet.

---

## Root-cause timeline (why each thing broke)

1. **"https error" / page won't load** → Brave holding a stale mkcert CA
   (running since before the CA was regenerated). Fix: restart Brave.
2. **Traefik lost https** → the shared Traefik had been recreated from a
   bare http-only config at some point; restored the websecure + cert
   config. (I initially, wrongly, concluded devpods "never had https" —
   it doesn't natively, but the user's mkcert layer does.)
3. **LinkedIn "Connect" failed** → Unipile trial instance died (503
   `no_client_session`). Fix: new paid instance creds (`:18524`).
   Gotcha: `docker restart` does **not** re-read compose `env_file`; must
   **recreate** (`devpods up`) to pick up `.env` changes.
4. **"Failed to fetch" / "not authenticated" on settings** → app URLs
   were http, browser on https → every authed `GET` CORS-blocked (only
   `OPTIONS` reached the api). Fix: app URLs → https (compose + .env).
5. **[OPEN] dashboard shows logged-out** → client-side Clerk session,
   likely http↔https origin mismatch. See "The one open issue".

---

## Key facts / gotchas for whoever continues

- **Scheme must be consistent end-to-end.** Browser origin, CORS
  `FRONTEND_URL`, and `NEXT_PUBLIC_API_URL` must all be **https** (they
  are now). Backend CORS = exact match on `FRONTEND_URL`
  (`backend/cmd/api/main.go:156 AllowedOrigins: []string{FRONTEND_URL}`).
- **`docker restart` ≠ env reload.** Container keeps creation-time env.
  Use `devpods up` to apply `.env` / compose changes.
- **Brave loads its cert store once at startup.** After any mkcert CA
  change, restart Brave.
- **`*.localhost` resolution:** works via glibc's built-in `.localhost`
  shortcut (curl) and Chromium's internal resolver (Brave). systemd-
  resolved is **not** running on this box; DNS is Tailscale
  (`100.100.100.100`) — irrelevant for `.localhost`.
- **Pre-existing unrelated warning:** api logs
  `CLERK_WEBHOOK_SECRET is set but invalid (illegal base64 ...)` — the
  Clerk *webhook* endpoint returns 503, but this does **not** affect
  session auth or the dashboard. Separate cleanup.

## Quick verification commands

```sh
# scheme consistency
docker exec devpod-magiklead__mvp-api-1 env | grep '^FRONTEND_URL='
docker exec devpod-magiklead__mvp-frontend-1 env | grep '^NEXT_PUBLIC_API_URL='

# CORS over https (expect access-control-allow-origin: https://mvp...)
curl -sk -D - -o /dev/null -X OPTIONS https://localhost:443/api/v1/settings \
  -H 'Host: api-mvp.magiklead.localhost' \
  -H 'Origin: https://mvp.magiklead.localhost' \
  -H 'Access-Control-Request-Method: GET' | grep -i access-control-allow-origin

# watch authed requests while the user clicks (expect 200, not 401)
docker logs -f devpod-magiklead__mvp-api-1 2>&1 | grep -aE 'GET|POST|401|403'

# Unipile instance alive + hosted-auth mints
source ~/.config/devpods/magiklead/.env.backend
curl -s -o /dev/null -w '%{http_code}\n' -H "X-API-KEY: $UNIPILE_API_KEY" "$UNIPILE_DSN/api/v1/accounts"   # 200
```
