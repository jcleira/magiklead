# Handoff — 2026-09-25: Sales Navigator search, the LinkedIn campaign page, and the next step

For the next session. Read this first, then `issues.md` and
[#11](./issues/11-campaign-creation-manual-start.md). The 2026-09-24
handoff (its task is done) is in git history.

## Founder decisions (locked — do not ask again)

- **No PDL.** Sourcing must cost nothing per prospect. `PDL_API_KEY` is
  commented out in `~/.config/devpods/magiklead/.env.backend` (backup
  `.env.backend.bak-20260925`). Do not turn it back on, and do not spend
  a credit. The PDL code stays, off by config.
- **A lead is valid for the smoke only with a LinkedIn profile.**
- **Lead source = a Sales Navigator people search through Unipile**, run
  as the smoke account. The founder buys Sales Navigator for
  `jose-corral-084bb6425`.
- **The connection note is optional** (pitch in DM 1 when it is empty).
- "With LinkedIn profile" is on when `/leads` opens.
- Earlier ones still hold: bind by DB insert; all fixes in one PR on
  `magiklead-mvp`; the smoke campaign targets the LLC plays.

## Next task: verify Sales Navigator, then the founder runs #11

1. **Wait for the founder:** Sales Navigator active on
   `jose-corral-084bb6425`.
2. **Check that Unipile sees it** (a read, no LinkedIn traffic), from
   inside the api container:
   `curl -s -H "X-API-KEY: $UNIPILE_API_KEY" "$UNIPILE_DSN/api/v1/accounts/_gtbOQ8dQ3aYT3bsPKptJA"`
   → `connection_params.im.premiumFeatures` must list Sales Navigator.
   On 2026-09-25 it was `[]` (`premiumId: null`). If Unipile does not
   pick it up, the founder may need Reconnect in the Unipile dashboard.
3. **One live search (consented: 10 results)** through the product
   path, as the founder: `POST /api/v1/leads/linkedin-search` with the
   law-firm play (titles `Managing Partner`, industries
   `Legal Services`, locations `United States`, size `51-200`). Check:
   - HTTP 200; results have real names and `linkedin.com/in/<slug>`
     URLs; `hidden` is small;
   - each result is a person with a `linkedin_url` identifier, a current
     job (title, company) and a location; one `linkedin_searches` row;
   - the id lookups use `REGION` and `SALES_INDUSTRY` — confirm Unipile
     accepts them for Sales Navigator.
   If the item shape differs from the documented example
   (`unipile_search_test.go`, `salesNavPage`), fix the parser in
   `backend/internal/linkedin/unipile/unipile_search.go` and add the
   real (redacted) shape as a test case.
4. **The founder runs #11 in the UI:** `/leads` → LinkedIn Sales
   Navigator → search → Save → `/campaigns/new-linkedin` (note optional)
   → Create → **Add saved leads** → read the note and DMs on the page →
   runbook §7 clean-standing check and the warm-up gate → **Start**.

## What changed on 2026-09-25 (PR #8)

| Commit | Change |
|---|---|
| `343cdb7` | Campaign leads list shows the person's LinkedIn URL; `AddLeads` skips a person with no profile on a LinkedIn campaign and counts real inserts; Start, Pause and the leads list check the tenant (any tenant could start another's campaign by id) |
| `a69e89a` | Saved leads carry `linkedin_url`, title and company |
| `411e85f` | `with_linkedin` search filter; the 90-day freshness rule applies only when PDL is on |
| `9f8bcd8` | Empty note → plain invite (worker trims); API rejects an empty DM |
| `f361b64` | A refused invite or DM waits 24 h (was: again every 60 s); after a refusal the account sends no more that tick |
| `403dfa4` | `POST /leads/linkedin-search` (Sales Navigator via Unipile); `linkedin_searches` table (migration 31, applied on the smoke DB); 250 profiles per account per rolling 24 h; the LinkedIn write-through reuses a person's company |
| `69ddd75` | A refused search says why (Unipile's `detail`) |
| `b500c7a` | Frontend: two search sources, profile links everywhere, Saved columns, the add-leads picker, the LinkedIn draft page (steps + Start with a confirm), the sequence view, the lead preview; editor: optional note, DM 1 "sent when they accept" |
| `6521c79` | Frontend: no search before Clerk loads; Start step not dimmed |
| `c7f5652` | e2e flow 18 adds its lead through the picker in a real browser (`@clerk/testing` 2.2.37) |
| `2b4a532` | CI: MinIO removed — quay.io now denies its images too; nothing in the suite uses S3 |

Tests: each backend change has an integration or unit test that fails
on the old code (checked against a `git archive` of `49f103d`). Full
backend suite green except the known `TestRegisterCreatesThreeSources…`.
Flows 02, 06, 07, 17, 18 and 20 passed on a local stack; flow 13 needs
Anthropic and was left to CI.

## State at handoff

- **Smoke pod:** all services up (air runs the worktree, so the code
  above is live). `stripe-listen` exits 1 (placeholder key, not needed).
  Migration 31 applied. PDL off (the api logs
  `PDL_API_KEY not set — lead search falls through to canonical-only mode`).
  `APP_URL` = the tunnel.
- **Unipile / LinkedIn:** DSN `https://api67.unipile.com:19786`; account
  `_gtbOQ8dQ3aYT3bsPKptJA` = `jose-corral-084bb6425`, app row `active`,
  **free** (no Premium, no Sales Navigator yet). The three
  `magiklead-smoke-*` webhooks are registered.
- **Founder tenant** `be594ede-…`: unchanged by today's work — 5 email
  drafts (LLC plays), 0 saved leads, 0 LinkedIn campaigns,
  0 `linkedin_events`. The live check's draft and saved lead were
  deleted.
- **Live LinkedIn activity today:** 2 id lookups + 1 classic search
  (10 results, all hidden) and 1 account read. No invite, no DM.

## Known issues (not fixed)

- **PDL names are lowercase** ("zachary lowe", "managing partner"), so
  `{{first_name}}` renders "zachary". Sales Navigator names are cased.
  Do not put the old PDL people in a campaign without a fix.
- Sales Navigator ids (`ACwAA…`) are not member ids (`ACoAA…`). The
  worker resolves the member id from the profile URL at invite time:
  one profile view per invite (Unipile guidance: ~100/day).
- Pause, then Start, does not resume leads already in a sequence
  (Pause clears `next_send_at`; `ActivateCampaignLeads` resets only
  `queued`). A paused campaign has no Resume button.
- `cmd/unipile-webhooks` test stub still returns `id` (client reads
  `webhook_id`); CI runs only e2e.
- The devpods repo module `compose/modules/minio.yml` uses
  `minio/minio:latest` and `minio/mc:latest`; both registries now deny
  them. A new pod on a clean machine fails; cached pods work.
- `CLERK_WEBHOOK_SECRET` is not valid base64 (the Clerk webhook 503s).
- No real `account.connected` callback has reached the app.
- The `/leads` description box filters nothing (database search only).

## Guardrails

- **Never** `devpods seed` or `devpods down` on the smoke pod.
  `devpods up` is safe.
- **Never** run `cmd/seed` or integration tests on the smoke DB
  (`processLinkedInQueue` acts on every due lead it reaches). Use the
  throwaway Postgres below.
- **No PDL**, no credit spend.
- A LinkedIn search runs as the smoke account: keep live searches to
  what the founder consented to.
- Do not `source` `.env.backend` (unquoted values with spaces). `grep`
  single variables.

## How to

- **SQL on the smoke DB:**
  `docker exec devpod-magiklead__smoke-postgres-1 psql -U devpod -d devpod -c '<sql>'`
- **Act as the founder (60 s JWT):** inside the api container,
  `POST https://api.clerk.com/v1/sessions {"user_id":"user_3DIGNMGwSJFoQg7l40AvZO71HXp"}`,
  then `POST /v1/sessions/<id>/tokens/magiklead-backend` (double-quoted
  `Authorization: Bearer $CLERK_SECRET_KEY`). Then
  `curl -k https://api-smoke.magiklead.localhost/api/v1/...`.
- **Apply a new migration on the smoke DB:**
  `docker exec devpod-magiklead__smoke-api-1 go run ./cmd/migrate`.
- **Integration tests:** `docker run -d --rm --name magiklead-testdb -e POSTGRES_USER=devpod -e POSTGRES_PASSWORD=devpod -e POSTGRES_DB=devpod -p 127.0.0.1:55439:5432 --tmpfs /var/lib/postgresql/data postgres:17-alpine`,
  then from `backend/`:
  `DATABASE_URL='postgres://devpod:devpod@127.0.0.1:55439/devpod?sslmode=disable' go run ./cmd/migrate`
  and `DATABASE_URL=… go test -tags=integration ./...`.
- **e2e on a local stack (not the smoke pod):** the throwaway Postgres
  above (+ `go run ./cmd/seed` on it), `redis:7-alpine` on
  `127.0.0.1:56379`, the api binary on `:18080`
  (`FRONTEND_URL=http://localhost:13000`, `CLERK_SECRET_KEY`,
  `UNSUBSCRIBE_SIGNING_SECRET`), and the frontend built from a copy of
  `frontend/` (`pnpm@11 install --frozen-lockfile`, `next build`,
  `next start -p 13000`, `NEXT_PUBLIC_API_URL=http://localhost:18080`).
  Playwright needs `CLERK_PUBLISHABLE_KEY` (UI flows),
  `E2E_API_URL`/`E2E_FRONTEND_URL`, `DATABASE_URL`, and `psql` on PATH
  (this host has none: a shim that runs `docker exec -i magiklead-testdb psql …` works).
- **Commit and push:** commit on `magiklead-smoke`, push
  `HEAD:refs/heads/magiklead-mvp` over HTTPS with gh's credential
  helper and the longer-prefix `insteadOf` override (SSH signing fails).
- **CI:** `gh pr checks 8 --repo jcleira/magiklead`. Watch every push
  to the end and report the result.
