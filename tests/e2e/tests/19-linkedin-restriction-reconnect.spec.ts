// Flow 19: LinkedIn account restriction → reconnect (issue #8).
//
// AFK substitute: LinkedIn restricting an account, and a human re-running
// the hosted-auth wizard, can't be driven headlessly — so we simulate the
// two Unipile webhooks the production system reacts to (account.status with
// a restriction, then account.connected for the reconnect), carrying the
// same signed metadata and the static Unipile-Auth header the real webhook
// receives, and
// assert each state transition via the DB and the list endpoint the Settings
// page renders. The browser UI is verified by that list endpoint (the data
// the restricted-status + Reconnect control bind to) per this suite's
// API/DB-driven convention; the page rendering itself is plain React over
// that payload.

import { test, expect } from '@playwright/test';
import crypto from 'node:crypto';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar, exec } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

const API_BASE = process.env.E2E_API_URL ?? 'http://api-mvp.magiklead.localhost';
// Must match the devpod's UNIPILE_WEBHOOK_SECRET (.env.backend).
const WEBHOOK_SECRET = process.env.UNIPILE_WEBHOOK_SECRET ?? 'test-unipile-webhook-secret-localdev-0000';

function b64url(input: string | Buffer): string {
  return Buffer.from(input).toString('base64url');
}

// mintMetadata reproduces backend/pkg/jwt (HS256, fixed header literal)
// so the webhook's jwt.Decode round-trips the tenant + user binding.
function mintMetadata(tenantID: string, userID: string, expUnix: number): string {
  const header = b64url('{"alg":"HS256","typ":"JWT"}');
  const payload = b64url(JSON.stringify({ tenant_id: tenantID, user_id: userID, exp: expUnix }));
  const signingInput = `${header}.${payload}`;
  const sig = crypto.createHmac('sha256', WEBHOOK_SECRET).update(signingInput).digest();
  return `${signingInput}.${b64url(sig)}`;
}

// postWebhook posts a raw Unipile webhook. Auth is the static Unipile-Auth
// header (value == UNIPILE_WEBHOOK_SECRET) real Unipile sends; pass a wrong
// auth to exercise the 401 path.
function postWebhook(body: string, auth: string = WEBHOOK_SECRET): Promise<Response> {
  return fetch(`${API_BASE}/api/v1/webhooks/unipile`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Unipile-Auth': auth },
    body,
  });
}

test('flow 19: restrict → last_error → list surfaces it → reconnect gate open → reconnect → active', async ({}, testInfo) => {
  const client = await newClient('flow19');
  const acctId = `acc_flow19_${crypto.randomUUID().slice(0, 8)}`;
  try {
    // Bootstrap the tenant, then resolve the ids the signed metadata binds.
    await client.get('/api/v1/settings');
    const tenantId = resolveTenantID(client.identity.userId);
    const userUUID = queryScalar(`SELECT id FROM users WHERE clerk_id='${client.identity.userId}';`);

    const exp = 4102444800; // 2100-01-01, comfortably in the future
    const meta = mintMetadata(tenantId, userUUID, exp);
    const connected = JSON.stringify({ status: 'CREATION_SUCCESS', account_id: acctId, name: meta });

    // 19a: connect the account so there is a live row to restrict (issue #1).
    expect((await postWebhook(connected)).status).toBe(200);
    expect(queryScalar(`SELECT status FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('active');

    // 19b: LinkedIn restricts it — an account.status webhook carrying an
    // ERROR flips status→restricted and records the provider reason in
    // last_error (the webhook detection source of issue #8 AC #1).
    const restricted = JSON.stringify({
      AccountStatus: { account_id: acctId, account_type: 'LINKEDIN', message: 'ERROR' },
    });
    expect((await postWebhook(restricted)).status).toBe(200);
    expect(queryScalar(`SELECT status FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('restricted');
    expect(queryScalar(`SELECT last_error FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('ERROR');

    // 19c: replay the same status webhook → still restricted, one row
    // (idempotent — the UPDATE is a no-op the second time, issue #8 AC #1).
    expect((await postWebhook(restricted)).status).toBe(200);
    expect(queryScalar(`SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('1');
    expect(queryScalar(`SELECT status FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('restricted');

    // 19d: the list endpoint the Settings page binds to surfaces the
    // restricted status — what the row's red dot + "Reconnect" control read
    // (issue #8 AC #5).
    const listRes = await client.get('/api/v1/linkedin/accounts');
    expect(listRes.status).toBe(200);
    const list = (await listRes.json()) as Array<{ unipile_account_id: string; status: string }>;
    expect(list.find((a) => a.unipile_account_id === acctId)?.status).toBe('restricted');

    // 19e: the reconnect control's auth-url is NOT blocked by the free-tier
    // gate while restricted — a restricted account no longer occupies the
    // one-account slot, so the user can re-run the connect flow. (Against the
    // dummy Unipile DSN the mint then fails with a 5xx, which is *past* the
    // gate — the point is it is not the 403 linkedin_account_limit an active
    // account hits. Contrast flow 16e.)
    const authRes = await client.get('/api/v1/linkedin/auth-url');
    expect(authRes.status).not.toBe(403);

    // 19f: reconnect — replaying account.connected upserts the same row back
    // to active and clears last_error (issue #8 AC #4).
    expect((await postWebhook(connected)).status).toBe(200);
    expect(queryScalar(`SELECT status FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('active');
    expect(queryScalar(`SELECT last_error IS NULL FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('t');

    await captureArtefact(testInfo, { tenantId });
  } finally {
    // Belt-and-suspenders: the tenant delete cascades linkedin_accounts
    // (FK ON DELETE CASCADE), but drop the row explicitly too so a partial
    // run never strands a stray account the platform-wide ticks would pick up.
    exec(`DELETE FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`);
    await cleanupTenant(client);
  }
});
