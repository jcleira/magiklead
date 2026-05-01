-- Pending GDPR erasure requests awaiting email confirmation.
-- Plan §T15: splits the public erasure endpoint into request + confirm
-- so identity verification happens via a signed token mailed to the
-- requester, instead of trusting whatever body arrives at the API.

CREATE TABLE privacy_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash       BYTEA NOT NULL UNIQUE,
    email            TEXT,
    linkedin_url     TEXT,
    name             TEXT,
    company          TEXT,
    reason           TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ NOT NULL,
    confirmed_at     TIMESTAMPTZ,
    deleted_persons  INT
);

CREATE INDEX idx_privacy_requests_expires
    ON privacy_requests(expires_at)
    WHERE confirmed_at IS NULL;
