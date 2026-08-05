// Flow 16: LinkedIn connect via Unipile hosted-auth.
//
// AFK substitute: the real hosted-auth wizard needs a human LinkedIn
// login, so we simulate Unipile's account.connected webhook — carrying
// the same signed metadata token and static Unipile-Auth header the
// production webhook receives — and assert the account lands `active`. In
// the devpod UNIPILE_API_KEY is a dev dummy (enables the connect routes)
// and UNIPILE_WEBHOOK_SECRET signs the metadata token and is the value of
// the Unipile-Auth header.

import { test, expect } from '@playwright/test';
import crypto from 'node:crypto';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar } from '../helpers/db.js';
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

test('flow 16: webhook connect → active → idempotent → free-tier gate → disconnect', async ({}, testInfo) => {
  const client = await newClient('flow16');
  const acctId = `acc_flow16_${crypto.randomUUID().slice(0, 8)}`;
  try {
    // Bootstrap the tenant, then resolve the ids the signed metadata binds.
    await client.get('/api/v1/settings');
    const tenantId = resolveTenantID(client.identity.userId);
    const userUUID = queryScalar(`SELECT id FROM users WHERE clerk_id='${client.identity.userId}';`);

    const exp = 4102444800; // 2100-01-01, comfortably in the future
    const meta = mintMetadata(tenantId, userUUID, exp);
    const connected = JSON.stringify({ status: 'CREATION_SUCCESS', account_id: acctId, name: meta });

    // 16a: account.connected webhook → 200; the row lands active, bound
    // to the tenant recovered from the signed metadata (AC #4).
    const r1 = await postWebhook(connected);
    expect(r1.status).toBe(200);
    expect(queryScalar(`SELECT status FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('active');
    expect(queryScalar(`SELECT tenant_id FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe(tenantId);

    // 16b: replay the SAME webhook → still exactly one row (AC #3 idempotent).
    const r2 = await postWebhook(connected);
    expect(r2.status).toBe(200);
    expect(queryScalar(`SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('1');

    // 16c: the connect callback authenticates by its SIGNED METADATA, not the
    // static Unipile-Auth header. The real hosted-auth notify_url arrives
    // header-less (fix 9db4491), so a connect with a wrong header but valid
    // metadata is still accepted and stays idempotent — no second row.
    const wrongHeader = await postWebhook(connected, 'wrong-secret');
    expect(wrongHeader.status).toBe(200);
    expect(queryScalar(`SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('1');

    // ...but the HS256 signature is the real gate: a connect whose metadata
    // signature doesn't verify is acknowledged (200, so Unipile stops
    // retrying) yet binds nothing.
    const forgedAcct = `acc_flow16_forged_${crypto.randomUUID().slice(0, 8)}`;
    const parts = mintMetadata(tenantId, userUUID, exp).split('.');
    const forgedMeta = `${parts[0]}.${parts[1]}.${b64url('not-a-valid-signature')}`;
    const forged = JSON.stringify({ status: 'CREATION_SUCCESS', account_id: forgedAcct, name: forgedMeta });
    const forgedRes = await postWebhook(forged, 'wrong-secret');
    expect(forgedRes.status).toBe(200);
    expect(queryScalar(`SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id='${forgedAcct}';`)).toBe('0');

    // 16d: /linkedin/accounts lists the connected account with status (AC #5).
    const listRes = await client.get('/api/v1/linkedin/accounts');
    expect(listRes.status).toBe(200);
    const list = (await listRes.json()) as Array<{ unipile_account_id: string; status: string }>;
    expect(list.map((a) => a.unipile_account_id)).toContain(acctId);
    expect(list.find((a) => a.unipile_account_id === acctId)?.status).toBe('active');

    // 16e: free-tier gate — a second connect attempt is refused 4xx and
    // writes no second row (AC #6). The dummy api key makes the route
    // Configured, so the request reaches the count gate (not a 503).
    const authRes = await client.get('/api/v1/linkedin/auth-url');
    expect(authRes.status).toBeGreaterThanOrEqual(400);
    expect(authRes.status).toBeLessThan(500);
    expect(queryScalar(`SELECT count(*) FROM linkedin_accounts WHERE tenant_id='${tenantId}';`)).toBe('1');

    // 16f: disconnect → 204 and the row is gone (AC #5). The Unipile
    // disconnect call against the dummy DSN fails but is tolerated, so
    // the local row still deletes.
    const rowId = queryScalar(`SELECT id FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`);
    const delRes = await client.del(`/api/v1/linkedin/accounts/${rowId}`);
    expect(delRes.status).toBe(204);
    expect(queryScalar(`SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id='${acctId}';`)).toBe('0');

    await captureArtefact(testInfo, { tenantId });
  } finally {
    await cleanupTenant(client);
  }
});
