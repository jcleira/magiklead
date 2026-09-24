# Handoff — 2026-09-24: LinkedIn smoke state and the next task

For the next session. Read this first, then `issues.md` and
[#11](./issues/11-campaign-creation-manual-start.md).

## Next task: a LinkedIn profile link on every lead

Founder request (2026-09-24): "I see the leads, but I want that they
have a link to the LinkedIn profile."

The founder has 0 saved leads, so they looked at the `/leads` search
results. Confirm the view with the founder before you build.

### What exists

- **Search results** (`frontend/src/app/(app)/leads/page.tsx:347`) show
  a "LinkedIn ↗" link when the API returns `linkedin_url`.
  `POST /api/v1/leads/search` reads it from the person's `linkedin_url`
  identifier (`attachLinkedIn`, `backend/internal/handler/leads_search.go`).
- **Campaign leads tab** (`frontend/src/app/(app)/campaigns/[id]/page.tsx:399`)
  shows a link when the row has `linkedin_url`.

### What is missing

1. **Most people have no URL.** Counts in the smoke DB on 2026-09-24:

   | Source | People | With `linkedin_url` | Fresh (≤ 90 days) |
   |---|---|---|---|
   | Ingest (Crunchbase, SEC, Wikidata, …) | 29,583 | 14 | 0 |
   | PDL | 33 | 8 | 33 |

   A search with titles only does not require fresh rows, so it shows
   ingest people, and nearly all of them have no URL. The 25 PDL people
   from August have no URL either: they came in before `c5f1747`. They
   get one when PDL returns them again.
2. **The Saved tab has no LinkedIn data.** `tenantLeadResponse`
   (`backend/internal/handler/tenant_leads.go`) and `SavedLead`
   (`frontend/src/lib/leads-api.ts`) carry only `person_id`, status,
   notes, date and name. The LinkedIn campaign adds its leads from this
   saved list (`campaigns/new-linkedin/page.tsx`: "Add saved LinkedIn
   prospects from the campaign once it's created").
3. **The campaign leads tab cannot show it for LinkedIn leads.**
   `ListCampaignLeads` (`backend/queries/campaign_leads.sql`) selects
   `l.linkedin_url` from the legacy `leads` table
   (`LEFT JOIN leads l ON l.id = cl.lead_id`). The LinkedIn rail adds
   leads by `person_id` (`AddPersonToCampaign`), so `lead_id` is NULL
   and the link is always empty. The invite queries already read the
   person's URL with an `li` CTE (`GetDueLinkedInInviteLeads`); reuse
   that shape with `COALESCE(li.linkedin_url, l.linkedin_url)`.
4. **Blocker: no screen adds saved leads to a campaign.** The LinkedIn
   editor says "Add saved LinkedIn prospects from the campaign once it's
   created", but no page calls `POST /api/v1/campaigns/{id}/leads`
   (`CampaignHandler.AddLeads`, body `{"person_ids": [...]}`). Only e2e
   flow 18 uses it, through the API
   (`tests/e2e/tests/18-linkedin-campaign.spec.ts:101`). So the founder
   cannot do #11 step 2 ("attach the saved leads") in the UI.

### Proposed work (confirm scope with the founder)

1. **Add saved leads to a campaign** (the blocker): a picker on the
   campaign page that lists the saved leads with their LinkedIn link and
   calls `POST /api/v1/campaigns/{id}/leads`. For a LinkedIn campaign,
   disable a lead with no `linkedin_url`. The leads stay `queued`, and
   nothing sends until **Start** (gate fixed in `6f55d5a`).
2. `ListCampaignLeads`: read the person's `linkedin_url` identifier, and
   fall back to the legacy column. Run `sqlc generate` in `backend/`.
3. Saved tab: add `linkedin_url` (and title + company, so the list is
   useful) to `tenantLeadResponse`, `SavedLead` and the `SavedTab` table.
4. Search tab: make it clear which people the LinkedIn rail can invite.
   For example, a "LinkedIn profile only" filter, or LinkedIn-ready rows
   first. The worker skips a lead without `linkedin_url`
   (`processLinkedInQueue`), so such a lead must not look invitable.
5. Optional, costs credits: fill in URLs for PDL people without one via
   PDL enrichment (1 credit per person). Ask the founder first.

### Acceptance criteria

- The founder can add saved leads to a LinkedIn campaign in the UI.
  The leads land `queued`, and nothing sends before **Start**.
- Every lead that the LinkedIn rail can invite shows a working link to
  its canonical profile URL (`https://www.linkedin.com/in/<slug>`) in
  the search results, the Saved tab and the campaign leads tab.
- A lead with no URL is marked as not invitable on LinkedIn.
- Tests: an integration test for `ListCampaignLeads` with a
  person-based lead, and a handler test for the saved-leads response.
  Each must fail on the old code. The e2e flow 18 adds its leads
  through the new UI, not only the API.
- Live check on the smoke pod: save one PDL person with a URL, see the
  link in the Saved tab, add it to a test LinkedIn draft, and see the
  link in its leads tab. Then delete that draft (there is no delete
  endpoint: use SQL, leads first) — do not click **Start**.

## State at handoff

### Smoke pod

- Worktree `~/Code/workspaces/workspace-magiklead-smoke/magiklead`,
  branch `magiklead-smoke`, pod `smoke`. App
  `https://smoke.magiklead.localhost`, API
  `https://api-smoke.magiklead.localhost`, tunnel
  `https://magiklead-smoke.magikshot.com`.
- All services are up, the worker too. `stripe-listen` exits 1:
  `STRIPE_SECRET_KEY` is a placeholder (Stripe is not needed).
- The pod runs the code in the worktree (air). A code edit is live
  as soon as you save it.

### Unipile and the LinkedIn account

- The subscription lapsed and was renewed. New workspace: DSN
  `https://api67.unipile.com:19786` and a new key, both in
  `~/.config/devpods/magiklead/.env.backend` (old file:
  `.env.backend.bak-20260923`, mode 600).
- Account `_gtbOQ8dQ3aYT3bsPKptJA` = the dedicated LinkedIn account
  `jose-corral-084bb6425` (warm-up since 2026-07-29). Unipile status
  `OK`. The app row `e576cdf6…` is `active`. It was bound by a DB
  insert (founder decision: no second LinkedIn login). No real
  `account.connected` callback has reached the app yet.
- The three `magiklead-smoke-*` webhooks are registered and enabled in
  the new workspace, at the tunnel.

### PR #8 (open, CI green)

<https://github.com/jcleira/magiklead/pull/8>, head `magiklead-mvp` →
base `argos/t14-admin-backend`. Last green e2e run:
<https://github.com/jcleira/magiklead/actions/runs/35969117651>.

| Commit | Change |
|---|---|
| `6f55d5a` | Invites and DMs send only for `active` campaigns (drafts and paused campaigns sent before) |
| `f288817` | PDL query ORs the values inside each filter; a PDL 404 is an empty result |
| `c5f1747` | PDL write-through stores the canonical `linkedin_url` and rebuilds a hidden location |
| `868148f` | A partial page is topped up from PDL; the description no longer reaches PDL |
| `4519e7a` | e2e CI pulls MinIO from `quay.io` (Docker Hub denies `minio/minio`) |
| docs | #06, #11, issues.md; `617e43d` (2026-09-22 restriction record) |

### Founder decisions (locked — do not ask again)

- Bind the relinked account by DB insert.
- All fixes in one PR, on the `magiklead-mvp` branch.
- Delete the fixture draft `a9608560…` (done).
- Prospect source: save the LinkedIn URL from PDL results (done).
- Delete the `cmd/seed` test data from the smoke DB (done: 26 people,
  7 companies, 5 saved leads).
- Top up a partial search page from PDL (done).
- **The smoke campaign targets the LLC plays** (generated 2026-09-24),
  not magikshot play #1.

### Data in the founder tenant

Tenant `be594ede-09be-4bb5-aa39-787710a1ec49`, Clerk user
`user_3DIGNMGwSJFoQg7l40AvZO71HXp`.

- Plays: 5 LLC plays (2026-09-24) and the older magikshot plays.
- Campaigns: 5 **email** drafts from onboarding, one per LLC play. They
  send nothing (drafts, no email account). "Mid-Market Law Firms…" holds
  3 law-firm people from **Discover Leads** (legacy website scrape,
  guessed emails). There is no LinkedIn campaign yet.
- 0 saved leads, 0 `linkedin_events`, 0 Gmail or SMTP accounts.

### #11 status

- Step 0 (reconnect): done by DB bind. Step 0.2 (runbook §7 clean
  standing) is the founder's check and is still open.
- Next: the founder searches and saves in `/leads`, then writes the
  LinkedIn campaign in `/campaigns/new-linkedin` and saves it as a
  draft. Nothing sends until **Start**.
- **Blocked at step 2** until a screen can add saved leads to a
  campaign (see the next task above).

## Guardrails

- **Never** `devpods seed` or `devpods down` on the smoke pod.
  `devpods up` is safe.
- **Never** run `cmd/seed` on the smoke pod: its test people fill real
  searches.
- **Never** run integration tests against the smoke DB:
  `processLinkedInQueue` acts on every due lead in the DB it reaches.
  Use a throwaway `postgres:17-alpine` (below).
- PDL bills per record returned. A search that the DB cannot fill costs
  up to 25 credits. The next-page button can cost 25 credits and add
  nobody (the PDL module does not page). 10 credits were used on
  2026-09-24.
- Do not `source` `.env.backend`: lines 63 and 65 hold unquoted values
  with spaces. `grep` single variables.
- PDL rejects any clause with a `query` key (`match` with
  `{query, operator}`, `query_string`). Use `match_phrase`.

## How to

- **SQL on the smoke DB:**
  `docker exec devpod-magiklead__smoke-postgres-1 psql -U devpod -d devpod -c '<sql>'`
- **Act as the founder (60 s JWT):** run the two Clerk calls inside the
  api container, so `$CLERK_SECRET_KEY` stays there:
  `POST https://api.clerk.com/v1/sessions {"user_id":"user_3DIGNMGwSJFoQg7l40AvZO71HXp"}`,
  then `POST /v1/sessions/<id>/tokens/magiklead-backend`. Use a
  double-quoted `Authorization: Bearer $CLERK_SECRET_KEY` header. Then
  `curl -k https://api-smoke.magiklead.localhost/api/v1/...`.
- **Integration tests:**
  1. `docker run -d --rm --name magiklead-testdb -e POSTGRES_USER=devpod -e POSTGRES_PASSWORD=devpod -e POSTGRES_DB=devpod -p 127.0.0.1:55439:5432 --tmpfs /var/lib/postgresql/data postgres:17-alpine`
  2. From `backend/`: `DATABASE_URL='postgres://devpod:devpod@127.0.0.1:55439/devpod?sslmode=disable' go run ./cmd/migrate`
  3. `DATABASE_URL=… go test -tags=integration ./...`. Expect one
     failure that is already on the base branch:
     `cmd/unipile-webhooks` `TestRegisterCreatesThreeSourcesThenKeepsThem`.
- **Recreate only the api after an env change** (from the worktree):
  `set -a; . ./.devpod.env; set +a`, then
  `docker compose -p devpod-magiklead__smoke -f $M/postgres.yml -f $M/redis.yml -f $M/minio.yml -f $M/stripe.yml -f $M/mailhog.yml -f devpod/compose.yml --project-directory . up -d --no-deps api`
  with `M=~/Code/repositories/devpods/compose/modules`.
- **Commit and push:** commit on `magiklead-smoke`, then push
  `HEAD:refs/heads/magiklead-mvp` (fast-forward). If SSH fails, push
  over HTTPS with gh's credential helper and a longer-prefix `insteadOf`
  override. The `magiklead-mvp` worktree directory is gone
  (`git worktree list` shows it as prunable).
- **CI:** `gh pr checks 8 --repo jcleira/magiklead`. Watch every push to
  the end and report the result.

## Open items (not fixed)

- `cmd/unipile-webhooks` test stub still returns `id`; `6d59b77` changed
  the client to read `webhook_id`. CI runs only e2e.
- Pause, then Start, does not resume leads already in a sequence
  (Pause clears `next_send_at`; `ActivateCampaignLeads` resets only
  `queued` leads). Step-0 invites do resume.
- The `/leads` description box filters nothing. A follow-up could fill
  titles, industries, locations and size from the plays instead.
- The devpods repo module `compose/modules/minio.yml` uses
  `minio/minio:latest` and `minio/mc:latest`. A new pod on a clean
  machine fails.
- `CLERK_WEBHOOK_SECRET` is not valid base64 (the Clerk webhook returns
  503).
- No real `account.connected` callback has reached the app.
