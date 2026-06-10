# 03 — Author a LinkedIn campaign + add prospects

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [02 — Source LinkedIn prospects](./02-source-linkedin-prospects.md)

## What to build

A tenant creates a LinkedIn campaign, authors a sequence (a
connection-request note as step 0, then ordered follow-up DMs with day
delays), and adds saved LinkedIn prospects to it — landing
`campaign_leads` rows queued for the LinkedIn engine. No sending yet.

End-to-end:
- New `campaigns.channel` column (`email`|`linkedin`; existing campaigns
  default `email`) routes the campaign to the LinkedIn tick.
- The sequence is stored in the existing `campaigns.linkedin_sequence`
  JSONB (already a column). A step = `{ step, delay_days, body }` — no
  subject; step 0's body is the connection note (enforce LinkedIn's
  ~300-char limit), steps 1+ are DMs. Personalization tokens
  `{{first_name}}` / `{{company}}` / `{{title}}` reuse the email
  renderer.
- Adding saved prospects (canonical persons) creates `campaign_leads`
  rows (person_id, status `queued`).
- A sequence-builder UI for LinkedIn steps.

## Acceptance criteria

- [ ] Migration adds `campaigns.channel` defaulting to `email`; existing
  campaigns unaffected.
- [ ] Creating a LinkedIn campaign persists `channel='linkedin'` and a
  `linkedin_sequence` with a step-0 note + ≥1 DM step; the 300-char note
  limit is validated (4xx on overflow). Assert via API + DB.
- [ ] Adding saved prospects creates `campaign_leads` rows with
  `person_id`, `status='queued'`; assert via DB.
- [ ] The personalization renderer fills first_name/company/title for
  the LinkedIn step shape (unit test).
- [ ] Sequence-builder UI: add/edit/reorder the note + DM steps with
  delays (Playwright happy path).

## Modules touched

- `campaigns` schema (`channel`) + repository.
- Campaign handler — accept/validate `linkedin_sequence` and the
  LinkedIn step shape.
- Reuse the personalization renderer from `internal/worker/sender.go`.
- Frontend campaign sequence builder (LinkedIn variant).

## Test prior art

- `internal/worker/sender_test.go` — personalization token rendering.
- Existing campaign handler tests; the `campaigns.go` AddLeads /
  tenant_leads add-prospect flow.

## Out of scope

- Any sending or pacing ([04](./04-send-connection-invite.md)). This
  slice only queues leads.
