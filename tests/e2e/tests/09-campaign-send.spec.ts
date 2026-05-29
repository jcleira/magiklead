// Flow 9: start campaign → worker sends step 1.
// AFK substitute: stub gmail_accounts row, observe the worker's
// 60s tick attempting to send and classify the failure correctly
// as 'token_expired'. The real-creds upgrade (real Gmail send →
// real gmail_message_id capture) lights up with E2E_REAL_GMAIL=1.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { exec, queryRows, queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

test('flow 9: worker tries to send, classifies token_expired correctly', async ({}, testInfo) => {
  test.setTimeout(180_000); // 60s tick + scaffolding.
  const client = await newClient('flow9');
  try {
    // Bootstrap a campaign with a sequence.
    await client.post('/api/v1/websites/analyze', { url: 'https://magikshot.com' });
    client.identity.jwt = await client.identity.mintFreshJWT();
    const plays = (await (await client.post('/api/v1/plays/generate', {})).json()) as Array<{
      id: string;
    }>;
    client.identity.jwt = await client.identity.mintFreshJWT();
    const camp = (await (
      await client.post('/api/v1/campaigns', { play_id: plays[0].id, name: 'E2E flow 9' })
    ).json()) as { id: string };
    client.identity.jwt = await client.identity.mintFreshJWT();
    await client.post('/api/v1/sequences/generate', { play_id: plays[0].id, campaign_id: camp.id });

    const tenantId = resolveTenantID(client.identity.userId);
    const userUUID = queryScalar(`SELECT id FROM users WHERE clerk_id='${client.identity.userId}';`);

    // Stub Gmail account.
    const gAccId = queryScalar(
      `INSERT INTO gmail_accounts (tenant_id,user_id,email,access_token,refresh_token,token_expiry) VALUES ('${tenantId}','${userUUID}','e2e-flow9-sender@gmail.com','stub-access','stub-refresh',NOW() + INTERVAL '1 hour') RETURNING id;`,
    );
    exec(`UPDATE campaigns SET gmail_account_id='${gAccId}' WHERE id='${camp.id}';`);

    // Find 1 fixture lead with a verified email + add to campaign.
    const personId = queryScalar(
      `SELECT p.id FROM persons p JOIN emails e ON e.person_id=p.id WHERE e.verified_at IS NOT NULL LIMIT 1;`,
    );
    client.identity.jwt = await client.identity.mintFreshJWT();
    await client.post('/api/v1/tenant_leads', { person_id: personId });
    await client.post(`/api/v1/campaigns/${camp.id}/leads`, { person_ids: [personId] });

    // Start campaign — ActivateCampaignLeads sets status=active + next_send_at=NOW().
    const startRes = await client.post(`/api/v1/campaigns/${camp.id}/start`, {});
    expect(startRes.status).toBe(200);

    // Wait for worker's 60s tick.
    await new Promise((resolve) => setTimeout(resolve, 70_000));

    // Inspect email_events — must have a 'failed' row with token_expired reason.
    const events = queryRows(
      `SELECT ee.event_type, ee.metadata->>'reason' FROM email_events ee JOIN campaign_leads cl ON cl.id=ee.campaign_lead_id WHERE cl.campaign_id='${camp.id}';`,
    );
    const failedWithTokenExpired = events.some(
      ([type, reason]) => type === 'failed' && reason === 'token_expired',
    );
    expect(failedWithTokenExpired, `events: ${JSON.stringify(events)}`).toBe(true);

    await captureArtefact(testInfo, { tenantId });

    // Cleanup the stub gmail_account FK first so account DELETE doesn't error.
    exec(`UPDATE campaigns SET gmail_account_id=NULL WHERE id='${camp.id}';`);
    exec(`DELETE FROM gmail_accounts WHERE id='${gAccId}';`);
  } finally {
    await cleanupTenant(client);
  }
});
