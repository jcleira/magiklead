# 04 — Unipile hygiene: rotate API key, verify clean account slate

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately. Must complete before
the day-one connect ceremony (#06).

## What to build

Start the smoke from clean credentials and a clean account slate
(PRD user story 15). Two parts, both operator-driven with
evidence-based verification:

1. **Rotate the Unipile API key.** The current key leaked into a
   session transcript during the June spike. The spike waived
   rotation on 2026-07-07 (it never reached the repo or git
   history), but the PRD re-decides: rotate before the smoke. The
   rotation itself happens in the Unipile dashboard (human step —
   see Handoff); then the new key replaces `UNIPILE_API_KEY` in
   `~/.config/devpods/magiklead/.env.backend` (user-owned, shared by
   all pods' api + worker), pods restart, and both directions are
   verified: the old key is dead, the new key works.

2. **Verify the personal account is gone.** The founder's personal
   LinkedIn was disconnected at Unipile during spike cleanup —
   verify it stayed gone (and disconnect it if present), so that
   after #06 the new founder-identity account is the **only**
   connected account. The subscription's minimum tier covers up to
   10 accounts; the point is slate cleanliness, not capacity.

Verification is pure curl against the Unipile REST API (no in-app
tooling needed):

```
curl -H "X-API-KEY: $KEY" "$UNIPILE_DSN/api/v1/accounts"
```

— 200 with the new key, 401 `missing_credentials` with the old one
(the error shape the spike captured). Note there is no Go wrapper
for `GET /api/v1/accounts`; raw curl is the expected path.

## Acceptance criteria

> **Rotation waived 2026-07-25** — founder decided to keep the existing
> key ("it's fine, use the existing key"). The leaked-key concern was
> raised and explicitly overruled. The three rotation-dependent criteria
> below are therefore N/A, not passed: the retained key is live *by
> design*. Only the clean-slate + live-config half of #04 is asserted,
> by `../verify-slate.sh` (6/6 green 2026-07-25).

- [x] ~~New key active~~ → key in use (`sha256 dcf1e148…`, unchanged)
      returns 200. No new key: rotation waived.
- [x] ~~Old key dead~~ → **N/A, waived.** No rotation means no old key;
      the retained key is intentionally still live. Not a passing
      "dead key" — the criterion no longer applies.
- [x] ~~`.env.backend` carries the new key; pods restarted~~ → no key
      change, so no restart. The part that still matters is verified:
      api healthy on both pods and the public webhook route gates (POST
      with a wrong `Unipile-Auth` header → 401, proving Unipile config
      is live). 2026-07-25.
- [x] Unipile accounts list shows **zero** connected accounts (or
      provably none belonging to the founder's personal profile)
      before #06 — output captured as day-0 evidence in the docs
      folder (redact ids as needed).
      → 2026-07-24: the premise was wrong. The personal account was
      *not* still gone — `publicIdentifier: jcleira` was connected
      2026-07-09 (after the spike cleanup) and bound `active` on the
      mvp pod. Founder approved disconnecting it;
      `DELETE /api/v1/accounts/…` → 200 `AccountDeleted`, list now
      `total_count: 0`. Evidence: [../day0-evidence.md](../day0-evidence.md).
- [x] Rotation *waived* 2026-07-25 (founder: use existing key); noted
      next to this row in `issues.md` and in #06's
      `Key rotation confirmed:` field.

## Modules touched

- `~/.config/devpods/magiklead/.env.backend` (user-owned secrets —
  never committed).
- `docs/2026-07-13-magikshot-linkedin-smoke/` — evidence capture.
- No code.

## Test prior art

None — verification is curl + captured output, per the PRD's
"evidence-based rather than vibes" rule. The 401 error shape is
documented in
`docs/2026-06-25-finish-unipile-integration/spike-captures.md`, and
was re-confirmed live on 2026-07-24:
`{"status":401,"type":"errors/missing_credentials",…}`.

## Verification harness

Rotation was waived (see the banner under Acceptance criteria), so the
rotation-specific harness (`verify-rotation.sh`) and the pre-rotation
key stash were removed. The retained checks live in one re-runnable
script:

```
docs/2026-07-13-magikshot-linkedin-smoke/verify-slate.sh
```

It asserts the key in `.env.backend` returns 200 (AC1), `total_count: 0`
(AC4), and both pods' `/health` 200 + webhook-gate 401 (AC3). Exit 0 =
green. It never prints key material, only the sha256 prefix. Last run
2026-07-25: **6 passed, 0 failed**.

History: a `verify-rotation.sh` was written first (it additionally
asserted a changed fingerprint and a dead old key, and stashed the
day-0 key at `~/.config/devpods/magiklead/unipile-prerotation-key.secret`
to keep the dead-key check provable). Its pre-rotation dry run was 6/2 —
failing exactly the rotation checks, proving the harness worked. When
the founder waived rotation, that script and the stash (a redundant copy
of a still-live secret) were both removed.

## Out of scope

- Connecting the new account — #06.
- Webhook registrations (they authenticate with
  `UNIPILE_WEBHOOK_SECRET`, not the API key, and are created in
  #06 via #03's CLI; rotating the API key does not invalidate
  them).

## Handoff

When the dashboard steps are done and the ACs above pass, BEFORE
flipping this row in `issues.md`, post this block to the user:

- **URL / artefact to visit**: Unipile dashboard (key management +
  connected accounts) — rotate the API key, confirm no personal
  account is connected.
- **Action required**: perform the rotation; place the new key in
  `~/.config/devpods/magiklead/.env.backend` (`UNIPILE_API_KEY`).
- **Where to record the decision**:
  - In `issues.md` (rotation date next to this row), AND
  - In [06 — Day-one connect ceremony](./06-day-one-connect-ceremony.md)
    under its `Key rotation confirmed:` field.

Issue #06 MUST NOT start until that field is filled.
