# 01 — Phase 0 unblock fixes

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately.

## What to build

Three small follow-ups from the working-MVP smoke walk on
2026-05-07. The first two are gaps that let bad state through api
startup without complaining; the third is a process hardening note.

1. **`CLERK_SECRET_KEY` format validator at api startup.** Today the
   startup check is "non-empty" — the placeholder value
   `sk_test_YOUR_CLERK_SECRET_KEY_HERE` passed startup and the
   backend got as far as serving requests before Clerk SDK rejected
   the token. Add a format check that rejects anything that does not
   match `^sk_(test|live)_[A-Za-z0-9]{20,}$` (Clerk's documented
   shape) so the api fails fast with a clear message instead of
   crashing later in request handling.

2. **`S3_ENDPOINT` scheme handling unit test.** Devpods injects
   `http://minio:9000` (with `http://` scheme); the MinIO Go SDK's
   `New()` only accepts host:port. The fix landed on the branch
   (uncommitted: `backend/internal/storage/s3.go`) — strip the
   scheme before calling `New()`, default `useSSL=false` for
   `http://` and `useSSL=true` for `https://`. Add a unit test that
   exercises both schemes, no-scheme, and the malformed-URL path.

3. **Snapshot regeneration on data-changing operations.** Process
   note (no code): document in `devpod/seeds/README.md` (or wherever
   the snapshot lifecycle lives) that any operation that mutates
   canonical data — running an ingest source, bulk fixture import,
   or schema migration with backfill — must finish with
   `devpods seed regenerate` before sign-off. The working-MVP smoke
   walk lost 29k+ canonical persons when the devpod was destroyed
   because the seed snapshot pre-dated the ingest run.

## Acceptance criteria

- [ ] api startup rejects placeholder `CLERK_SECRET_KEY` values with
      a clear error message (not a generic "missing env var"); regex
      check on `^sk_(test|live)_[A-Za-z0-9]{20,}$`.
- [ ] Unit test for the clerk format check covers: valid test key,
      valid live key, placeholder, empty string, malformed prefix.
- [ ] `backend/internal/storage/s3.go` fix is committed; unit test
      exercises `http://host:9000`, `https://host:9000`, `host:9000`
      (no scheme), and a malformed URL with garbage.
- [ ] `devpod/seeds/README.md` (or equivalent) documents the
      regenerate-after-mutation rule.
- [ ] Pending uncommitted fixes from the 2026-05-07 smoke walk are
      committed: storage `s3.go`, `next.config.ts` `allowedDevOrigins`,
      and any `.env.backend` template touch-ups. Verify with
      `git status` clean afterwards.

## Modules touched

- `backend/cmd/api/main.go` — clerk key format validator.
- `backend/internal/storage/s3.go` — scheme handling fix (already
  written, needs commit + tests).
- `backend/internal/storage/s3_test.go` — new unit tests.
- `devpod/seeds/README.md` — process note.
- `frontend/next.config.ts` — `allowedDevOrigins` (already written,
  needs commit).

## Test prior art

- `backend/internal/middleware/admin_test.go` — pattern for testing
  a small validation function with table-driven cases.
- `backend/internal/storage/s3_roundtrip_test.go` — pattern for
  storage tests with skip-when-env-var-missing.

## Out of scope

- The devpods CLI compose-tree locator regression from 2026-05-07
  is tracked in the upstream devpods repo, not here.
- Lazy tenant bootstrap (working-MVP issue #1) — remains on the
  working-MVP tracker; not duplicated here.
