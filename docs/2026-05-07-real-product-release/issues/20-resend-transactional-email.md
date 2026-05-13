# 20 — Resend transactional email

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#16 — Hetzner VPS + api online](./16-hetzner-vps-and-api.md)

## What to build

Switch the system transactional mailer (`internal/email`) from
MailHog (devpod) to Resend in production so emails like the GDPR
erasure confirmation, billing receipts, and operator alerts
actually arrive in real inboxes. The existing SMTP abstraction
handles the code path; this slice is configuration plus account
provisioning.

Per the PRD, this is for transactional mail only — customer
outbound (campaign sends) goes through each tester's connected
Gmail OAuth (see [#3](./03-gmail-oauth-send.md)), never through
Resend.

## Acceptance criteria

- [ ] Resend account exists; `magiklead.com` added and verified
      in the Resend dashboard.
- [ ] Production env file (`/etc/magiklead/.env.production` on
      the VPS) contains:
      `SYSTEM_SMTP_HOST=smtp.resend.com`,
      `SYSTEM_SMTP_PORT=587`, `SYSTEM_SMTP_USER=resend`,
      `SYSTEM_SMTP_PASS=<resend-api-key>`,
      `SYSTEM_SMTP_FROM=no-reply@magiklead.com`.
      `.env.production.example` documents the same keys with
      placeholder values.
- [ ] No code change to `backend/internal/email/sender.go`
      (existing SMTP abstraction handles Resend transparently).
- [ ] Trigger the erasure flow on production: submit a request,
      click the link in the *real* inbox (not MailHog), confirm
      flow completes successfully. The email arrives in the inbox
      (not spam) — definitive spam check is in
      [#21](./21-sender-domain-auth.md).
- [ ] Resend dashboard shows the message as "delivered" with no
      SMTP errors.

## Modules touched

- Transactional email integration (configuration only; no code
  change).

## Test prior art

- `backend/internal/handler/privacy_test.go` exercises erasure
  validation paths against a nil mailer; the production
  verification is manual end-to-end.

## Out of scope

- Customer outbound email (Gmail OAuth for campaign sends) —
  unrelated path; see [#3](./03-gmail-oauth-send.md).
- Sender domain DNS records (SPF/DKIM/DMARC) — see
  [#21](./21-sender-domain-auth.md). Resend's dashboard generates
  the DKIM record but adding it to DNS belongs to the next slice.
- Templated transactional email (multiple senders, branded HTML
  templates) — defer.
