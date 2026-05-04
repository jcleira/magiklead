# 13 — Email finder source (paid)

**Type**: HITL — operator chooses the provider, manages the subscription, holds the API key.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#3 Real ingest run](./03-real-ingest-run.md) — needs the canonical `persons` graph in place to enrich.

## What to build

Wire a paid email-finder service into the ingest pipeline so the `emails` table actually fills up for real exec rows. Issue #3 proved that free public sources (EDGAR Form 4, Wikidata SPARQL, TeamPages scrape) yield zero person-tied emails in 2026 — modern B2B SaaS hides them behind contact forms. Pattern-guessing is banned by `docs/2026-04-21-lead-database-architecture/research.md §3` because of bounce risk.

Paid finders solve this by aggregating real addresses from public web data + breach databases + contributor networks, and SMTP-verifying before returning. Effective per-real-email cost lands around $0.02–0.05 on B2B execs.

The integration shape: a new `cmd/enrich-emails` CLI analogous to `cmd/verify-emails`. It iterates over canonical persons that don't have a verified email yet, calls the chosen provider's API, and writes the result through the same `q.CreateEmail` path issue #3 wired up. Provider returns SMTP-verified, so `verification_method='<provider>-verified'` and `verified_at=now()` go in directly — no second-pass verification needed.

## Provider recommendation

**Findymail** ($99/mo Starter — see issue #3 conversation for full price comparison). Reasons:

- Pay-only-for-verified credit model (failed lookups don't burn credits) — best fit for one-shot enrichment of an existing graph.
- Native LinkedIn-URL → email lookup, which pairs directly with the LinkedIn ingest source if/when that lands separately.
- API on the entry tier, no sales-call required.
- Lowest claimed bounce rate (<2%) of the self-serve options.

Hunter.io Growth (€104/mo for 10k credits, ~28% hit rate) is the runner-up if Findymail's coverage on the actual list turns out poor. Apollo is cheapest per credit but the API is gated to Custom (sales-call) plans, which we'd want to avoid for v1.

## Acceptance criteria

- [ ] Provider chosen and account created. API key and any required webhook config in `~/.config/devpods/magiklead/.env.backend` (e.g. `FINDYMAIL_API_KEY=...`).
- [ ] New CLI at `backend/cmd/enrich-emails/main.go` that:
  - lists canonical persons missing a verified email (and joined to a current employment whose org has a `primary_domain` or person has a `linkedin_url` identifier — provider-dependent on which inputs work),
  - calls the provider's API with sensible batching and rate limiting,
  - writes results via `q.CreateEmail` with `verification_method='<provider>-verified'` and `verified_at=NOW()` when the provider returns verified, or `verification_method='<provider>-catchall'` / `'<provider>-unverifiable'` for the other outcomes,
  - logs per-batch hit rate and cumulative spend (credits consumed × known per-credit cost) so the operator can monitor cost.
- [ ] A small dry-run mode (`--limit 10`) that does not consume credits but prints what it *would* call, so the operator can sanity-check before spending.
- [ ] Unit tests for the provider client (mock HTTP, fixture responses for verified / catchall / unverifiable / not-found / rate-limited / 4xx auth fail).
- [ ] One real run against ~100 persons; result quality eyeballed by the operator.
- [ ] `POST /api/v1/leads/search` for "VP Sales" with `with_email=true` returns real persons whose emails are now populated and verified.
- [ ] Cost per real email logged for the post-PRD record.

## Modules touched

- New CLI: `backend/cmd/enrich-emails/`.
- New provider client package: `backend/internal/leads/<provider>/` (e.g. `internal/leads/findymail/`).
- No changes needed in `internal/ingest/resolver.go` — issue #3 already wired `fields["email"]` → `q.CreateEmail`. The CLI calls `q.CreateEmail` directly without going through the ingest source/resolver flow because we're enriching existing canonical rows, not ingesting new raw batches.
- No changes to `cmd/verify-emails` either — provider-verified rows already have `verified_at` populated, so they're skipped by `ListUnverifiedEmails`.

## Test prior art

- `backend/internal/ingest/sources/edgar_test.go`, `wikidata_test.go` — HTTP-mocked source tests showing the idiom: spin a `httptest.Server`, point the client at it, assert parsed records.
- `backend/internal/leads/smtp_verify.go` + the existing `cmd/verify-emails` for the "iterate over persons + write back via repository" pattern.

## Out of scope

- Multi-provider fallback (start with one provider; add fallback only if first hits coverage limits in production).
- Continuous re-enrichment as a scheduled job (v1 is one-shot; can land later as a worker task).
- Phone-number enrichment (most paid email finders also sell phones; v1 scope is email only).
- Pattern-guessing fallback for misses — explicitly banned by the architecture and the whole reason this issue exists.
- LinkedIn ingest source — that's a separate slice of work; this issue can run with EDGAR/Wikidata/TeamPages persons alone (org `primary_domain` is the input the provider uses).
