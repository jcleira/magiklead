-- Intentional no-op: the 030 up migration is a lossy one-way data rewrite.
--
-- After it runs, a row that was rewritten from 'linkedin' is byte-identical
-- to one the live search / seed wrote directly as 'linkedin_url', and the
-- original scheme-less spelling is not preserved. A faithful reverse is
-- therefore impossible: flipping every 'linkedin_url' row back to 'linkedin'
-- would over-capture rows that were never 'linkedin', and could not restore
-- the old "linkedin.com/<path>" value form. Roll back by restoring from a
-- pre-migration dump if this is ever truly needed.
SELECT 1;
