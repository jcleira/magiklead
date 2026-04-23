-- T08 cross-source dedup needs an order-insensitive person match so
-- "Musk Elon" (EDGAR format) and "Elon Musk" (Wikidata/CrunchBase
-- format) collapse into one canonical. A trgm index on canonical_name
-- is order-sensitive (trigrams carry word boundaries), so we store a
-- tokens-lowercased-and-alphabetically-sorted form alongside it.

ALTER TABLE persons ADD COLUMN normalized_name TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_persons_normalized_name_trgm
    ON persons USING GIN (normalized_name gin_trgm_ops);

-- Naive backfill — lowercases and strips punctuation. Going forward,
-- the resolver writes the sorted-tokens form on INSERT so new rows
-- match cross-format-name reliably. A later job can re-normalize
-- existing rows if we ever need cross-format matches on historical
-- data.
UPDATE persons
SET normalized_name = LOWER(REGEXP_REPLACE(canonical_name, '[^a-zA-Z0-9 ]', ' ', 'g'));
