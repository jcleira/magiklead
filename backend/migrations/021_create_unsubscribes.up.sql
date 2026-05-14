-- Issue #2: the unified "do not email" table. tenant_id NULL means a
-- global suppression (CAN-SPAM: a recipient unsubscribing from any
-- tenant should never be emailed again by anyone on the platform).
--
-- Unique key includes COALESCE on tenant_id because Postgres treats
-- NULL as not-equal-to-NULL for UNIQUE; without the COALESCE, a
-- single email could land in the table twice as "global". The
-- sentinel UUID is the documented all-zeros UUID per RFC 4122 §4.1.7
-- (nil UUID); no real tenant_id ever takes this value because
-- gen_random_uuid() never returns it.
--
-- reason vocabulary (free text in the column for forward-compat, but
-- writers should stick to these values):
--   manual                  — operator added by hand
--   list-unsub              — user clicked List-Unsubscribe header
--   spam-complaint          — Gmail feedback loop / spam report
--   reply                   — recipient replied, suppression module
--                             fans out from RecordReply
--   hard-bounce             — DSN 5.x.x classification
--   soft-bounce-threshold   — third consecutive soft bounce

CREATE TABLE unsubscribes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    reason      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uniq_unsubscribes_tenant_email
    ON unsubscribes (COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(email));

CREATE INDEX idx_unsubscribes_email
    ON unsubscribes (lower(email));
