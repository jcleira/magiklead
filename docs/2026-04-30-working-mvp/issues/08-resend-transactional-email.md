# 08 — Resend transactional email

**Type**: HITL — needs a Resend account + API key + sending-domain verification.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#4 — Hetzner VPS + domain + api online](./04-hetzner-vps-and-api.md)

## What to build

Switch the system transactional mailer (`internal/email`) from MailHog (devpod) to Resend in production so emails like the GDPR erasure confirmation actually arrive in real inboxes. The existing SMTP abstraction handles the code path; this slice is configuration plus account provisioning.

## Acceptance criteria

- [ ] Resend account exists; sending domain (e.g. `magiklead.com`) added and verified in the Resend dashboard.
- [ ] Production env file (`/etc/magiklead/.env.production` on the VPS) contains: `SYSTEM_SMTP_HOST=smtp.resend.com`, `SYSTEM_SMTP_PORT=587`, `SYSTEM_SMTP_USER=resend`, `SYSTEM_SMTP_PASS=<resend-api-key>`, `SYSTEM_SMTP_FROM=no-reply@<domain>`. `.env.production.example` documents the same keys with placeholder values.
- [ ] No code change to `backend/internal/email/sender.go` (existing SMTP abstraction handles Resend transparently).
- [ ] Trigger the erasure flow on production: submit a request, click the link in the *real* inbox (not MailHog), confirm flow completes successfully. The email arrives in the inbox (not spam) — definitive spam check is in [issue #9](./09-sender-domain-auth.md).
- [ ] Resend dashboard shows the message as "delivered" with no SMTP errors.

## Modules touched

- Transactional email integration (configuration only; no code change).

## Test prior art

- `backend/internal/handler/privacy_test.go` exercises erasure validation paths against a nil mailer; the production verification is manual end-to-end.

## Out of scope

- Customer outbound email (Gmail OAuth or customer SMTP for campaign sends) — explicitly untouched per PRD; that path is the customer's responsibility.
- Sender domain DNS records (SPF/DKIM/DMARC) — see [issue #9](./09-sender-domain-auth.md). Resend's dashboard generates the DKIM record but adding it to DNS belongs to the next slice.
- Templated transactional email (multiple senders, branded HTML templates) — defer.
