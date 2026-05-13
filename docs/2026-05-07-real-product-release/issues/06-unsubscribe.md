# 06 — Unsubscribe end-to-end

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#2 — Suppression module + schema migrations](./02-suppression-module.md), [#3 — Gmail OAuth real send](./03-gmail-oauth-send.md)

## What to build

RFC 8058 one-click unsubscribe plus the classic `mailto:` fallback
on every outbound campaign message. A public endpoint accepts the
one-click POST and records the unsubscribe through the suppression
module — to both the per-tenant suppression list and the global
suppression list (CAN-SPAM).

After this slice, every outbound message carries both headers,
Gmail's deliverability checks are satisfied, and recipients can
unsubscribe in one click — and once they do, they're never emailed
again from any tenant on the platform.

## Acceptance criteria

- [ ] Every outbound message via `gmail/sender` includes:
  - `List-Unsubscribe: <mailto:unsubscribe+<token>@<mail-domain>>, <https://<app-domain>/api/v1/public/unsubscribe?token=<token>>`,
  - `List-Unsubscribe-Post: List-Unsubscribe=One-Click`.
  - `<token>` is a signed JWT (HS256 with a backend secret)
    encoding the lead's email, tenant ID, and an expiry far enough
    out that suppression remains actionable (e.g. 5 years).
- [ ] New public endpoint
      `POST /api/v1/public/unsubscribe?token=<jwt>`:
  - validates the JWT signature and expiry,
  - calls `suppression.RecordUnsubscribe(ctx, email,
    &tenantID, reason='list-unsub')`, writing both to the
    per-tenant entry and a global entry (or whatever shape #2's
    module exposes — `RecordUnsubscribe` writes once and the
    `IsSuppressed` read considers both `(tenant_id, email)` and
    `(NULL, email)`).
  - returns a plain confirmation page (200 with a small HTML
    "You've been unsubscribed" body).
- [ ] `GET /api/v1/public/unsubscribe?token=<jwt>` returns the
      same confirmation page (some email clients pre-fetch on hover
      — RFC 8058 says POST is the one-click path, GET is a
      legitimate fallback).
- [ ] `mailto:unsubscribe+<token>@<mail-domain>` handler: there is
      no inbound-email-processing infra in this release. Document
      in the operator runbook that the `mailto:` channel is
      *advertised* (Gmail's deliverability gate requires both
      headers present) but inbound emails to that mailbox will pile
      up unanswered until a follow-up wires inbound parsing. Mark
      this as a known operational gap; PRD section "Out of scope"
      does NOT cover this but it's an honest small-scale call.
- [ ] Once unsubscribed for a given (tenant, email), the worker's
      next tick skips that lead via `suppression.IsSuppressed`.
- [ ] Unit tests for the public endpoint:
  - valid token: writes the unsubscribe row, returns 200,
  - expired token: returns 410 Gone,
  - tampered token: returns 401,
  - already-unsubscribed: idempotent — returns 200, no duplicate
    row.
- [ ] Manual local verification: send a campaign message to a real
      Gmail address, click the unsubscribe header in Gmail's UI
      (the "Unsubscribe" link Gmail surfaces from the `List-Unsubscribe`
      header), confirm next worker tick skips that lead.

## Modules touched

- `backend/internal/gmail/sender.go` — adds the two headers and
  encodes the JWT.
- `backend/internal/handler/` — new `public_unsubscribe.go` with
  the one-click endpoint (under
  `/api/v1/public/...` so no Clerk auth required).
- `backend/cmd/api/main.go` — registers the public route outside
  the auth middleware chain.
- `backend/internal/suppression/` — already exposes the
  `RecordUnsubscribe` write from #2; consumer here.

## Test prior art

- `backend/internal/handler/privacy.go` and `privacy_test.go` —
  the privacy-erasure flow is the same shape: public endpoint,
  signed token, idempotent write. Reuse the JWT helper if one
  already exists; create one in `pkg/` if not.
- `backend/internal/handler/tenant_leads_test.go` — handler
  validation-path testing pattern.

## Out of scope

- Inbound `mailto:` parsing — see acceptance criteria note. Tracked
  as an honest operational gap for a follow-up.
- Per-step unsubscribe rates dashboard — data is in `email_events`
  + `unsubscribes`; dashboard work lives in
  [#9](./09-campaign-metrics.md).
- Re-subscribe UI — a recipient who unsubscribed is permanently
  suppressed; a per-tenant override is out of scope.
- Spam-complaint feedback loops (FBL) — Gmail doesn't operate one
  in the public API; defer.
