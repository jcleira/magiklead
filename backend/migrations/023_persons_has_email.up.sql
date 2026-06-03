-- PDL's free tier obfuscates contact fields: work_email / emails come
-- back as a boolean presence flag (true = "a value exists you can't
-- see") instead of the address. has_email persists that presence
-- signal so the canonical search can surface emailable prospects — and
-- with_email can match them — even before a paid plan returns the
-- actual address. On a paid plan it is simply set whenever a real
-- address is written through. See issue #7.
ALTER TABLE persons
    ADD COLUMN has_email BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index supports the with_email gate (… OR p.has_email).
CREATE INDEX idx_persons_has_email ON persons(has_email) WHERE has_email = TRUE;
