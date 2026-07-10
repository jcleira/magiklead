-- Issue #6: person-keyed suppression for the LinkedIn rail. Email
-- suppression keys on the address; LinkedIn prospects have no email, so
-- a reply must suppress the *person* instead. We extend the existing
-- unsubscribes table rather than add a parallel one so IsSuppressed has a
-- single source of truth and the GDPR export keeps dumping one table.
--
--   - person_id (nullable): set on LinkedIn reply/manual rows; NULL on the
--     email rows that predate this slice.
--   - email becomes nullable: a person row carries no address. The CHECK
--     keeps every row keyed by at least one of (email, person_id).
--   - the existing email unique index needs no change — a NULL email is
--     distinct under a UNIQUE index, so person rows never collide there;
--     person idempotency is enforced by a separate partial unique index.
--
-- Only the reply / manual reasons apply on LinkedIn (no bounces).

ALTER TABLE unsubscribes
    ALTER COLUMN email DROP NOT NULL,
    ADD COLUMN person_id UUID REFERENCES persons(id) ON DELETE CASCADE;

ALTER TABLE unsubscribes
    ADD CONSTRAINT unsubscribes_email_or_person
    CHECK (email IS NOT NULL OR person_id IS NOT NULL);

-- Person-keyed idempotency: one suppression row per (tenant, person).
-- COALESCE mirrors the email index so a NULL tenant_id (global) collapses
-- to the same sentinel; partial so it only governs person rows.
CREATE UNIQUE INDEX uniq_unsubscribes_tenant_person
    ON unsubscribes (COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), person_id)
    WHERE person_id IS NOT NULL;

CREATE INDEX idx_unsubscribes_person ON unsubscribes (person_id) WHERE person_id IS NOT NULL;
