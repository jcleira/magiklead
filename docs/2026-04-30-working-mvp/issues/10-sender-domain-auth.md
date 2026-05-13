# 10 — Sender domain auth (SPF/DKIM/DMARC)

**Type**: HITL — needs DNS registrar credentials + Resend dashboard access.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#9 — Resend transactional email](./09-resend-transactional-email.md)

## What to build

Add SPF, DKIM, and DMARC TXT records to the project domain's DNS so system transactional emails (erasure confirmations etc.) authenticate cleanly and don't land in spam.

- **SPF** authorises Resend to send on behalf of the domain.
- **DKIM** (TXT record from Resend's dashboard) proves the message wasn't tampered with.
- **DMARC** starts in monitor mode (`p=none`) so misconfigurations don't bounce real mail before the operator sees aggregate reports.

## Acceptance criteria

- [ ] DNS contains `v=spf1 include:_spf.resend.com -all` (or whatever Resend's docs specify) on the apex.
- [ ] DNS contains the DKIM TXT record(s) Resend's dashboard generated. Resend's domain status flips to "verified" / "active" for DKIM.
- [ ] DNS contains a DMARC record at `_dmarc.<domain>`: `v=DMARC1; p=none; rua=mailto:dmarc@<domain>` (or operator's chosen reporting mailbox).
- [ ] `dig TXT <domain>`, `dig TXT resend._domainkey.<domain>` (or whichever selector Resend uses), `dig TXT _dmarc.<domain>` all return the expected records.
- [ ] [mail-tester.com](https://www.mail-tester.com) score for an email sent through the system mailer is ≥ 9/10, with no SPF/DKIM/DMARC failures.
- [ ] An erasure confirmation email sent to a Gmail address lands in the inbox, not spam. Headers in Gmail's "Show original" show `SPF: PASS`, `DKIM: PASS`, `DMARC: PASS`.

## Modules touched

- Operational only (DNS).

## Test prior art

- None — verification is by external tooling and live test.

## Out of scope

- Customer outbound deliverability (campaigns sent through customer Gmail or customer SMTP) — explicitly the customer's responsibility per PRD.
- DMARC enforcement (`p=quarantine` / `p=reject`) — defer until aggregate reports look clean for at least a week.
- BIMI / brand indicators — defer.
- MTA-STS / TLS-RPT — defer.
