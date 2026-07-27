-- Issue #07 — unify person LinkedIn identity on identifier_type
-- 'linkedin_url' + the canonical full-URL form
-- (internal/linkedin/liurl.Canonical).
--
-- The ingest resolver historically wrote person LinkedIn rows as
-- identifier_type 'linkedin' + a scheme-less "linkedin.com/<path>" value,
-- while every rail reader (lead search, GetDueLinkedInInviteLeads /
-- GetDueLinkedInDMLeads) filters on 'linkedin_url'. Ingest-created people
-- were therefore invisible to the LinkedIn rail. This rewrites the legacy
-- rows to the unified type + canonical value.
--
-- Canonical transform: liurl.Canonical of a legacy value equals
-- 'https://www.' || value, because those legacy values are always the
-- scheme-less "linkedin.com/<path>" form NormalizeLinkedInURL produced
-- (lowercase, no scheme, no www, no trailing slash). The concatenation
-- below is exactly that transform expressed in SQL — see
-- TestCanonicalMatchesMigrationConcat.
--
-- person_identifiers.identifier_value is globally UNIQUE, so a legacy row
-- whose canonical target already exists on another row (e.g. a live-search
-- 'linkedin_url' row for the same human) would violate the constraint on
-- UPDATE. Those legacy duplicates are dropped and the existing canonical
-- row kept; merging the two person rows they point at is out of scope.
-- This touches ONLY person_identifiers — org LinkedIn rows live in
-- organization_identifiers under 'linkedin' and are intentionally left.

-- 1. Drop legacy rows whose canonical target already exists elsewhere.
DELETE FROM person_identifiers AS legacy
WHERE legacy.identifier_type = 'linkedin'
  AND EXISTS (
      SELECT 1 FROM person_identifiers AS other
      WHERE other.identifier_value = 'https://www.' || legacy.identifier_value
        AND other.id <> legacy.id
  );

-- 2. Rewrite the survivors to the unified type + canonical value.
UPDATE person_identifiers
SET identifier_type  = 'linkedin_url',
    identifier_value = 'https://www.' || identifier_value
WHERE identifier_type = 'linkedin';
