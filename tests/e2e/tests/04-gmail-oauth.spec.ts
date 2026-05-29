// Flow 4: Gmail OAuth connect / disconnect / reconnect.
//
// AFK substitute: real OAuth requires a Google consent screen
// click, so we drive the API surface and simulate post-callback
// state via direct DB insert. The real-creds path lights up when
// E2E_REAL_GMAIL=1 with a pre-provisioned refresh token.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar, exec } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

test('flow 4: auth-url + stub-connect + disconnect + reconnect', async ({}, testInfo) => {
  const client = await newClient('flow4');
  try {
    // 4a: /gmail/auth-url returns a well-formed Google OAuth URL.
    const authUrlRes = await client.get('/api/v1/gmail/auth-url');
    expect(authUrlRes.status).toBe(200);
    const { url } = (await authUrlRes.json()) as { url: string };
    expect(url).toMatch(/^https:\/\/accounts\.google\.com\/o\/oauth2\/auth/);
    expect(url).toContain('scope=');
    expect(url).toContain('gmail.send');
    expect(url).toContain('gmail.readonly');

    // 4b: bootstrap tenant + insert a stub gmail_accounts row
    // (the AFK substitute for the OAuth callback).
    await client.get('/api/v1/settings');
    const tenantId = resolveTenantID(client.identity.userId);
    const userUUID = queryScalar(`SELECT id FROM users WHERE clerk_id='${client.identity.userId}';`);

    const gAccId = queryScalar(
      `INSERT INTO gmail_accounts (tenant_id,user_id,email,access_token,refresh_token,token_expiry) VALUES ('${tenantId}','${userUUID}','e2e-flow4@gmail.com','stub-access','stub-refresh',NOW() + INTERVAL '1 hour') RETURNING id;`,
    );
    expect(gAccId).toMatch(/^[0-9a-f]{8}-/);

    // 4c: /gmail/accounts lists the stub.
    const listRes = await client.get('/api/v1/gmail/accounts');
    expect(listRes.status).toBe(200);
    const list = (await listRes.json()) as Array<{ id: string; email: string }>;
    expect(list).toHaveLength(1);
    expect(list[0].email).toBe('e2e-flow4@gmail.com');

    // 4d: /settings reports the connected badge.
    const settingsRes = await client.get('/api/v1/settings');
    const settings = (await settingsRes.json()) as {
      email_accounts: Array<{ status: string; email: string }>;
    };
    expect(settings.email_accounts[0].status).toBe('connected');

    // 4e: disconnect.
    const delRes = await client.del(`/api/v1/gmail/accounts/${gAccId}`);
    expect(delRes.status).toBe(204);
    const afterDelete = (await (await client.get('/api/v1/settings')).json()) as {
      email_accounts: unknown[];
    };
    expect(afterDelete.email_accounts).toEqual([]);

    // 4f: reconnect (second stub).
    exec(
      `INSERT INTO gmail_accounts (tenant_id,user_id,email,access_token,refresh_token,token_expiry) VALUES ('${tenantId}','${userUUID}','e2e-flow4-v2@gmail.com','stub-access-2','stub-refresh-2',NOW() + INTERVAL '1 hour');`,
    );
    const reconnected = (await (await client.get('/api/v1/settings')).json()) as {
      email_accounts: Array<{ email: string }>;
    };
    expect(reconnected.email_accounts.map((a) => a.email)).toContain('e2e-flow4-v2@gmail.com');

    await captureArtefact(testInfo, { tenantId });
  } finally {
    await cleanupTenant(client);
  }
});

test('flow 4 (real creds): full Google OAuth round-trip', async () => {
  test.skip(
    process.env.E2E_REAL_GMAIL !== '1',
    'requires E2E_REAL_GMAIL=1 + a pre-provisioned operator refresh token',
  );
  // Real-creds path lands in #15 — placeholder until then.
});
