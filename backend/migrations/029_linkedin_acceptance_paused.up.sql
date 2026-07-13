-- The acceptance-rate circuit breaker (issue #7) flags an account whose
-- trailing-7-day connection-acceptance rate has fallen below the safe floor,
-- so the pacer pauses its invites and the UI can surface why. It is a
-- transient, self-clearing flag — distinct from status='restricted' (issue
-- #8), which is a hard provider-side stop — so it lives in its own column
-- and the account stays active/warming: still reconciled, still re-evaluated
-- each tick, so it resumes automatically once the rate recovers. The unused
-- acceptance_rate column (created with the table in 024) finally gets
-- populated alongside it, so the flag is explainable.
ALTER TABLE linkedin_accounts
    ADD COLUMN acceptance_paused BOOLEAN NOT NULL DEFAULT FALSE;
