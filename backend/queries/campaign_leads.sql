-- Legacy: add a row via the lead_id path. Kept while the pipeline
-- still runs; T07 deletes it.
-- name: AddLeadToCampaign :exec
INSERT INTO campaign_leads (campaign_id, lead_id, status)
VALUES ($1, $2, 'queued')
ON CONFLICT DO NOTHING;

-- New canonical path — add a saved person to a campaign.
-- name: AddPersonToCampaign :exec
INSERT INTO campaign_leads (campaign_id, person_id, status)
VALUES ($1, $2, 'queued')
ON CONFLICT DO NOTHING;

-- GetDueLeads resolves the contact fields from either the canonical
-- persons graph (when campaign_leads.person_id is set) or the legacy
-- leads row (lead_id). The sender reads a unified row shape regardless
-- of which path populated it.
--
-- For the canonical path we pull the best email per person (verified
-- wins, then fewest bounces, then oldest) and the current employment
-- for title/company.
--
-- The channel='email' guard keeps the email send loop off LinkedIn
-- campaigns. It matters from issue #5 on: an accepted LinkedIn lead
-- goes status='active' + next_send_at=NOW() (so the LinkedIn DM tick
-- picks it up), which would otherwise also match this query — and the
-- email sender, finding no email for a LinkedIn-only person, would
-- wrongly mark it 'exhausted'. The two rails select disjoint leads by
-- their campaign channel.
-- name: GetDueLeads :many
WITH best AS (
    SELECT DISTINCT ON (em.person_id)
        em.person_id,
        em.email,
        em.verified_at,
        em.is_catchall,
        em.bounce_count
    FROM emails em
    ORDER BY em.person_id,
             (em.verified_at IS NOT NULL) DESC,
             em.bounce_count ASC,
             em.created_at ASC
), current_emp AS (
    SELECT DISTINCT ON (emp.person_id)
        emp.person_id,
        emp.title,
        emp.organization_id
    FROM employments emp
    WHERE emp.is_current = TRUE
    ORDER BY emp.person_id, emp.start_date DESC NULLS LAST
)
SELECT
    cl.id,
    cl.campaign_id,
    cl.lead_id,
    cl.person_id,
    cl.status,
    cl.current_step,
    cl.next_send_at,
    COALESCE(b.email, l.email, '')::text             AS email,
    COALESCE(p.first_name, l.first_name, '')::text   AS first_name,
    COALESCE(p.last_name,  l.last_name,  '')::text   AS last_name,
    COALESCE(o.canonical_name, l.company, '')::text  AS company,
    COALESCE(ce.title, l.title)                      AS title
FROM campaign_leads cl
JOIN campaigns         c  ON c.id  = cl.campaign_id AND c.channel = 'email'
LEFT JOIN leads         l  ON l.id  = cl.lead_id
LEFT JOIN persons       p  ON p.id  = cl.person_id
LEFT JOIN best          b  ON b.person_id = cl.person_id
LEFT JOIN current_emp   ce ON ce.person_id = cl.person_id
LEFT JOIN organizations o  ON o.id = ce.organization_id
WHERE cl.next_send_at <= NOW()
  AND cl.status = 'active'
ORDER BY cl.next_send_at
LIMIT $1;

-- name: UpdateCampaignLeadStep :exec
UPDATE campaign_leads
SET current_step = $2, next_send_at = $3, last_sent_at = NOW(), status = $4
WHERE id = $1;

-- ListCampaignLeads returns the rows for one campaign with contact
-- fields resolved from either the canonical persons graph (preferred)
-- or the legacy leads table. Mirrors GetDueLeads's CTE structure so
-- that the UI displays the same unified shape.
-- name: ListCampaignLeads :many
WITH best AS (
    SELECT DISTINCT ON (em.person_id)
        em.person_id,
        em.email
    FROM emails em
    ORDER BY em.person_id,
             (em.verified_at IS NOT NULL) DESC,
             em.bounce_count ASC,
             em.created_at ASC
), current_emp AS (
    SELECT DISTINCT ON (emp.person_id)
        emp.person_id,
        emp.title,
        emp.organization_id
    FROM employments emp
    WHERE emp.is_current = TRUE
    ORDER BY emp.person_id, emp.start_date DESC NULLS LAST
)
SELECT
    cl.id,
    cl.campaign_id,
    cl.lead_id,
    cl.person_id,
    cl.status,
    cl.current_step,
    cl.next_send_at,
    cl.last_sent_at,
    cl.last_opened_at,
    cl.last_replied_at,
    cl.created_at,
    COALESCE(p.first_name,    l.first_name, '')::text   AS first_name,
    COALESCE(p.last_name,     l.last_name,  '')::text   AS last_name,
    COALESCE(b.email,         l.email,      '')::text   AS email,
    COALESCE(ce.title,        l.title)                  AS title,
    COALESCE(o.canonical_name, l.company,   '')::text   AS company,
    l.linkedin_url                                       AS linkedin_url
FROM campaign_leads cl
LEFT JOIN leads         l  ON l.id  = cl.lead_id
LEFT JOIN persons       p  ON p.id  = cl.person_id
LEFT JOIN best          b  ON b.person_id = cl.person_id
LEFT JOIN current_emp   ce ON ce.person_id = cl.person_id
LEFT JOIN organizations o  ON o.id = ce.organization_id
WHERE cl.campaign_id = $1
ORDER BY cl.created_at DESC;

-- name: CountCampaignLeadsByStatus :many
SELECT status, COUNT(*) as count FROM campaign_leads
WHERE campaign_id = $1
GROUP BY status;

-- name: ActivateCampaignLeads :exec
UPDATE campaign_leads SET status = 'active', next_send_at = NOW()
WHERE campaign_id = $1 AND status = 'queued';

-- name: PauseCampaignLeads :exec
UPDATE campaign_leads SET next_send_at = NULL
WHERE campaign_id = $1 AND status = 'active';

-- ListCampaignLeadsForExport dumps every campaign_lead row tied to a
-- tenant's campaigns, raw — no join resolution. Used by the GDPR
-- account-export endpoint (issue #11) to write campaign_leads.json
-- inside the user's archive.
-- name: ListCampaignLeadsForExport :many
SELECT cl.*
FROM campaign_leads cl
JOIN campaigns c ON c.id = cl.campaign_id
WHERE c.tenant_id = $1
ORDER BY cl.created_at;

-- MarkCampaignLeadBounced halts the sequence on a hard bounce. The
-- sender's GetDueLeads query filters WHERE status='active' so a
-- bounced row stops receiving sends on the very next tick. We do
-- not clear next_send_at — keeping the timestamp around is useful
-- for "when did we last try?" forensics.
-- name: MarkCampaignLeadBounced :exec
UPDATE campaign_leads
SET status = 'bounced'
WHERE id = $1;

-- GetDueLinkedInInviteLeads selects step-0 (connection-invite) leads for
-- LinkedIn campaigns that are ready to send now. A lead qualifies while
-- it is still at step 0 and either queued (never launched-activated) or
-- active+due — the LinkedIn launch path runs ActivateCampaignLeads just
-- like email, so both states appear. The recipient identifier is the
-- person's canonical linkedin_url; name/title/company are resolved for
-- note personalization (no email join — the LinkedIn rail never touches
-- emails). tenant_id rides along so the tick can pick the tenant's
-- connected account. Mirrors GetDueLeads's CTE shape.
-- name: GetDueLinkedInInviteLeads :many
WITH current_emp AS (
    SELECT DISTINCT ON (emp.person_id)
        emp.person_id,
        emp.title,
        emp.organization_id
    FROM employments emp
    WHERE emp.is_current = TRUE
    ORDER BY emp.person_id, emp.start_date DESC NULLS LAST
), li AS (
    SELECT DISTINCT ON (pi.person_id)
        pi.person_id,
        pi.identifier_value AS linkedin_url
    FROM person_identifiers pi
    WHERE pi.identifier_type = 'linkedin_url'
    ORDER BY pi.person_id
)
SELECT
    cl.id,
    cl.campaign_id,
    cl.person_id,
    cl.current_step,
    c.tenant_id,
    COALESCE(li.linkedin_url, '')::text  AS linkedin_url,
    COALESCE(p.first_name, '')::text     AS first_name,
    COALESCE(p.last_name,  '')::text     AS last_name,
    COALESCE(o.canonical_name, '')::text AS company,
    ce.title                             AS title
FROM campaign_leads cl
JOIN campaigns        c  ON c.id  = cl.campaign_id AND c.channel = 'linkedin'
LEFT JOIN persons       p  ON p.id  = cl.person_id
LEFT JOIN current_emp   ce ON ce.person_id = cl.person_id
LEFT JOIN organizations o  ON o.id = ce.organization_id
LEFT JOIN li               ON li.person_id = cl.person_id
WHERE cl.current_step = 0
  AND cl.status IN ('queued', 'active')
  AND (cl.next_send_at IS NULL OR cl.next_send_at <= NOW())
ORDER BY cl.created_at
LIMIT $1;

-- MarkLinkedInInviteSent parks a lead waiting for acceptance after its
-- step-0 invite goes out: status='awaiting_accept', next_send_at cleared
-- (acceptance, not a timer, advances it — issue #5), and the account +
-- Unipile invitation bound for reconciliation. current_step stays 0; the
-- first DM is step 1 and is scheduled only once the invite is accepted.
-- name: MarkLinkedInInviteSent :exec
UPDATE campaign_leads
SET status = 'awaiting_accept',
    next_send_at = NULL,
    last_sent_at = NOW(),
    linkedin_account_id = $2,
    linkedin_invitation_id = $3
WHERE id = $1;

-- MarkLinkedInAccepted is issue #5's acceptance transition: a parked
-- invite (awaiting_accept) becomes a live conversation. It flips the
-- lead to active, stamps accepted_at, schedules the first DM now
-- (next_send_at=NOW()) and moves to step 1 so the DM tick picks it up.
-- The WHERE gate on status='awaiting_accept' makes it idempotent: a
-- replayed acceptance webhook (or one for an already-advanced lead)
-- updates zero rows and RETURNING yields no row, so the caller writes
-- no duplicate 'accepted' event. The $1::text cast pins the param to a
-- plain string. Matched by the Unipile invitation id bound at send time.
-- name: MarkLinkedInAccepted :one
UPDATE campaign_leads
SET status = 'active',
    accepted_at = NOW(),
    next_send_at = NOW(),
    current_step = 1
WHERE linkedin_invitation_id = sqlc.arg(invitation_id)::text
  AND status = 'awaiting_accept'
RETURNING id;

-- GetLinkedInAwaitingAcceptByAccount returns the awaiting_accept leads for
-- one connected account paired with the prospect's cached Unipile member id
-- (issue #7) — the accept matcher's input. Each ~45-min reconcile lists the
-- account's current connections and flips every awaiting lead whose member id
-- now appears in that set. Only leads whose person already has a cached
-- linkedin_member_id identifier (resolved at invite time — issue #5) are
-- addressable, so the join is inner; a lead with no cached id simply waits.
-- Scoped to the lead's bound account so one account's connections never flip
-- another account's leads.
-- name: GetLinkedInAwaitingAcceptByAccount :many
SELECT
    cl.id,
    pi.identifier_value AS member_id
FROM campaign_leads cl
JOIN person_identifiers pi
    ON pi.person_id = cl.person_id
   AND pi.identifier_type = 'linkedin_member_id'
WHERE cl.status = 'awaiting_accept'
  AND cl.linkedin_account_id = sqlc.arg(account_id);

-- MarkLinkedInAcceptedByMember is issue #7's acceptance transition, keyed on
-- the Unipile member id the connections check matched rather than the
-- invitation id the (slow, 8-hour-lagged) notification carries. It flips
-- every awaiting_accept lead whose person owns member_id and whose invite was
-- sent from account_id: status→active, accepted_at stamped, first DM
-- scheduled now (next_send_at=NOW(), current_step=1) so the DM tick picks it
-- up. The awaiting_accept gate makes it idempotent — a re-run over an
-- already-flipped lead matches zero rows and RETURNING yields nothing, so the
-- caller writes no duplicate 'accepted' event. Scoped to account_id so a
-- member connected on one account never flips a lead bound to another.
-- RETURNING is :many because one person (member ids are unique) may sit in
-- more than one campaign against the same account; each returned lead gets
-- its own accepted event.
-- name: MarkLinkedInAcceptedByMember :many
UPDATE campaign_leads cl
SET status = 'active',
    accepted_at = NOW(),
    next_send_at = NOW(),
    current_step = 1
FROM person_identifiers pi
WHERE pi.person_id = cl.person_id
  AND pi.identifier_type = 'linkedin_member_id'
  AND pi.identifier_value = sqlc.arg(member_id)::text
  AND cl.linkedin_account_id = sqlc.arg(account_id)
  AND cl.status = 'awaiting_accept'
RETURNING cl.id;

-- GetDueLinkedInDMLeads selects accepted LinkedIn leads due for their
-- next direct message: active, past step 0, and due now. The lead is
-- already bound (issue #4/#5) to the account that sent its invite, so we
-- INNER JOIN linkedin_accounts to carry that account's unipile id — the
-- DM goes from the same account, into the same relationship. A lead whose
-- account was disconnected (linkedin_account_id nulled via ON DELETE SET
-- NULL) drops out of the join and is simply not messaged, which is the
-- safe direction. The la.status filter extends that pause to a restricted
-- or disconnected (but not yet row-deleted) account (issue #8): its
-- in-flight DM leads are held until it reconnects to active/warming, the
-- same health predicate pickLinkedInAccount applies on the invite tick.
-- linkedin_chat_id is NULL for the first DM (no chat exists until we start
-- one) and set thereafter. Recipient + name/title/company are resolved for
-- personalization, mirroring the invite query.
-- name: GetDueLinkedInDMLeads :many
WITH current_emp AS (
    SELECT DISTINCT ON (emp.person_id)
        emp.person_id,
        emp.title,
        emp.organization_id
    FROM employments emp
    WHERE emp.is_current = TRUE
    ORDER BY emp.person_id, emp.start_date DESC NULLS LAST
), li AS (
    SELECT DISTINCT ON (pi.person_id)
        pi.person_id,
        pi.identifier_value AS linkedin_url
    FROM person_identifiers pi
    WHERE pi.identifier_type = 'linkedin_url'
    ORDER BY pi.person_id
)
SELECT
    cl.id,
    cl.campaign_id,
    cl.person_id,
    cl.current_step,
    cl.linkedin_chat_id,
    c.tenant_id,
    la.unipile_account_id,
    COALESCE(li.linkedin_url, '')::text  AS linkedin_url,
    COALESCE(p.first_name, '')::text     AS first_name,
    COALESCE(p.last_name,  '')::text     AS last_name,
    COALESCE(o.canonical_name, '')::text AS company,
    ce.title                             AS title
FROM campaign_leads cl
JOIN campaigns         c  ON c.id  = cl.campaign_id AND c.channel = 'linkedin'
JOIN linkedin_accounts la ON la.id = cl.linkedin_account_id
LEFT JOIN persons       p  ON p.id  = cl.person_id
LEFT JOIN current_emp   ce ON ce.person_id = cl.person_id
LEFT JOIN organizations o  ON o.id = ce.organization_id
LEFT JOIN li               ON li.person_id = cl.person_id
WHERE cl.status = 'active'
  AND cl.current_step >= 1
  AND cl.next_send_at <= NOW()
  AND la.status IN ('active', 'warming')
ORDER BY cl.next_send_at
LIMIT $1;

-- GetLinkedInLeadByChatID maps a Unipile chat back to the campaign_lead it
-- belongs to — the key the inbound-message webhook and the reconcile poll
-- both use to attribute a reply. The chat id is recorded on the lead by the
-- first DM (issue #5). Returns no row for an unknown chat (a safe no-op).
-- name: GetLinkedInLeadByChatID :one
SELECT id FROM campaign_leads
WHERE linkedin_chat_id = sqlc.arg(chat_id)::text
LIMIT 1;

-- SuppressLinkedInLead halts a lead the DM tick's person-suppression gate
-- caught (the person replied or unsubscribed via another campaign_lead).
-- status='suppressed' drops it out of GetDueLinkedInDMLeads (which filters
-- status='active'), the LinkedIn analogue of the sender's suppressed flip.
-- name: SuppressLinkedInLead :exec
UPDATE campaign_leads
SET status = 'suppressed'
WHERE id = $1;

-- AdvanceLinkedInDMStep moves a lead forward after a DM goes out, the
-- LinkedIn analogue of UpdateCampaignLeadStep. It records the chat id
-- the send returned (so later steps thread into the same chat — issue
-- #6) without clobbering an existing one when the param is empty, and
-- advances the step/schedule/status the worker computed (next step's
-- jittered delay, or 'exhausted' when the sequence is done).
-- name: AdvanceLinkedInDMStep :exec
UPDATE campaign_leads
SET current_step = sqlc.arg(current_step),
    next_send_at = sqlc.arg(next_send_at),
    status = sqlc.arg(status),
    linkedin_chat_id = COALESCE(NULLIF(sqlc.arg(chat_id)::text, ''), linkedin_chat_id),
    last_sent_at = NOW()
WHERE id = sqlc.arg(id);

-- GetStaleLinkedInInvites returns connection invites that have sat
-- unaccepted past the withdrawal horizon (issue #7): LinkedIn leads still
-- parked awaiting_accept whose invite was sent more than 21 days ago
-- (last_sent_at is stamped at send by MarkLinkedInInviteSent and untouched
-- while parked, so it is the invite-sent time). It carries the Unipile
-- invitation id to cancel and the account's unipile id to cancel it from.
-- A lead whose account was disconnected (linkedin_account_id nulled via ON
-- DELETE SET NULL) drops out of the join — there is nothing to withdraw
-- from. LIMIT bounds each sweep.
-- name: GetStaleLinkedInInvites :many
SELECT
    cl.id,
    cl.linkedin_invitation_id,
    la.unipile_account_id
FROM campaign_leads cl
JOIN linkedin_accounts la ON la.id = cl.linkedin_account_id
WHERE cl.status = 'awaiting_accept'
  AND cl.linkedin_invitation_id IS NOT NULL
  AND cl.last_sent_at < NOW() - INTERVAL '21 days'
ORDER BY cl.last_sent_at
LIMIT $1;

-- MarkLinkedInNotAccepted closes a lead whose connection invite was
-- withdrawn after going unaccepted past the horizon (issue #7):
-- status='not_accepted', a terminal state that drops it out of every due
-- query. The awaiting_accept guard makes it idempotent and avoids a race
-- with a late acceptance — if the invite was accepted between the sweep's
-- SELECT and this UPDATE, the row is already 'active' and we leave it be.
-- name: MarkLinkedInNotAccepted :exec
UPDATE campaign_leads
SET status = 'not_accepted'
WHERE id = $1 AND status = 'awaiting_accept';

-- MarkLinkedInFailed closes a lead the resolver could not turn into a Unipile
-- member id for a permanent reason — the prospect's profile is gone, private,
-- or otherwise unresolvable (issue #5). 'failed' is a terminal free-text
-- status (no migration — the column has no enum) that drops the lead out of
-- every due query, so it is never retried. Transient resolve failures do NOT
-- come here: they leave the lead queued for the next tick.
-- name: MarkLinkedInFailed :exec
UPDATE campaign_leads
SET status = 'failed'
WHERE id = $1;
