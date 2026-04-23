-- T08's resolver re-asserts aliases on every merge, which piled up
-- duplicate "Elon Musk" rows when multiple Wikidata filings landed on
-- the same canonical. Adding a unique constraint lets the resolver use
-- ON CONFLICT DO NOTHING and keep the alias table clean.

-- Drop any existing duplicates before adding the constraint.
DELETE FROM person_aliases a
USING person_aliases b
WHERE a.id > b.id
  AND a.person_id = b.person_id
  AND a.alias = b.alias
  AND a.alias_type = b.alias_type;

DELETE FROM organization_aliases a
USING organization_aliases b
WHERE a.id > b.id
  AND a.organization_id = b.organization_id
  AND a.alias = b.alias
  AND a.alias_type = b.alias_type;

ALTER TABLE person_aliases
    ADD CONSTRAINT person_aliases_unique UNIQUE (person_id, alias, alias_type);

ALTER TABLE organization_aliases
    ADD CONSTRAINT organization_aliases_unique UNIQUE (organization_id, alias, alias_type);
