// Flow 20: LinkedIn campaign metrics + weekly capacity (issue #9) — the
// data behind the per-campaign LinkedIn dashboard and the capacity
// indicator. Seeds a linkedin_events funnel + a linkedin_accounts row
// with rolling counters, asserts the two endpoint contracts the UI
// renders (acceptance rate, reply rate, remaining weekly invites), then
// proves cross-tenant metrics access returns 404 (no existence leak).
//
// API status codes + direct DB seeding/reads — the house pattern (flows
// 13/18). The suite is API-driven; the UI itself (LinkedInMetricsSection)
// consumes exactly these two payloads.

import { test, expect } from '@playwright/test';
import crypto from 'node:crypto';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar, exec } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

interface LinkedInMetrics {
  invites_sent: number;
  accepted: number;
  acceptance_rate: number;
  dms_sent: number;
  replies: number;
  reply_rate: number;
}

interface Capacity {
  connected: boolean;
  status?: string;
  weekly_cap?: number;
  weekly_used?: number;
  weekly_remaining?: number;
}

test('flow 20: LinkedIn metrics + weekly capacity', async ({}, testInfo) => {
  test.setTimeout(120_000);
  const owner = await newClient('flow20');
  const other = await newClient('flow20other');
  const suffix = crypto.randomUUID().slice(0, 8);
  let tenantId = '';
  let campId = '';
  try {
    await owner.get('/api/v1/settings'); // lazy-bootstrap the tenant
    tenantId = resolveTenantID(owner.identity.userId);

    // A play to attach the campaign to (FK target only).
    const playId = queryScalar(
      `INSERT INTO plays (tenant_id, name, icp, search_query, channels) ` +
        `VALUES ('${tenantId}','Flow20 Play ${suffix}','{}'::jsonb,'{}'::jsonb, ARRAY['linkedin']::text[]) RETURNING id;`,
    );

    // A LinkedIn campaign via the real route (channel persisted + sequence).
    owner.identity.jwt = await owner.identity.mintFreshJWT();
    const createRes = await owner.post('/api/v1/campaigns', {
      play_id: playId,
      name: `Flow20 LI ${suffix}`,
      channel: 'linkedin',
      linkedin_sequence: [
        { step: 0, delay_days: 0, body: 'Hi {{first_name}}, open to connecting?' },
        { step: 1, delay_days: 2, body: 'Thanks for connecting, {{first_name}}!' },
      ],
    });
    const createBody = await createRes.text();
    expect(createRes.status, createBody).toBe(201);
    campId = (JSON.parse(createBody) as { id: string }).id;

    // Seed the funnel in one pass: 4 leads → 4 invites, 2 accepted, 2 DMs,
    // 1 reply. So acceptance 2/4 = 0.5 and reply 1/2 = 0.5. row_number
    // assigns the per-lead roles deterministically; the counts are
    // independent of its ordering.
    exec(
      `WITH np AS (
         INSERT INTO persons (canonical_name, normalized_name, has_email)
         SELECT 'Flow20 ' || g || ' ${suffix}', 'flow20 ' || g || ' ${suffix}', false
         FROM generate_series(1,4) AS g
         RETURNING id
       ),
       nl AS (
         INSERT INTO campaign_leads (campaign_id, person_id, status)
         SELECT '${campId}', id, 'active' FROM np
         RETURNING id
       ),
       ranked AS (SELECT id, row_number() OVER () AS rn FROM nl),
       inv AS (
         INSERT INTO linkedin_events (campaign_lead_id, event_type, step)
         SELECT id, 'invite_sent', 0 FROM ranked
       ),
       acc AS (
         INSERT INTO linkedin_events (campaign_lead_id, event_type, step)
         SELECT id, 'accepted', 1 FROM ranked WHERE rn <= 2
       ),
       dm AS (
         INSERT INTO linkedin_events (campaign_lead_id, event_type, step)
         SELECT id, 'dm_sent', 1 FROM ranked WHERE rn <= 2
       )
       INSERT INTO linkedin_events (campaign_lead_id, event_type, step)
       SELECT id, 'replied', 1 FROM ranked WHERE rn = 1;`,
    );

    // 20a: per-campaign metrics — counts + the two derived rates.
    owner.identity.jwt = await owner.identity.mintFreshJWT();
    const mRes = await owner.get(`/api/v1/campaigns/${campId}/linkedin-metrics`);
    expect(mRes.status).toBe(200);
    const m = (await mRes.json()) as LinkedInMetrics;
    expect(m.invites_sent).toBe(4);
    expect(m.accepted).toBe(2);
    expect(m.dms_sent).toBe(2);
    expect(m.replies).toBe(1);
    expect(m.acceptance_rate).toBeCloseTo(0.5, 5);
    expect(m.reply_rate).toBeCloseTo(0.5, 5);

    // Seed a connected account: fully warmed (60d), 30 invites used this
    // week → cap 100, used 30, remaining 70.
    exec(
      `INSERT INTO linkedin_accounts ` +
        `(tenant_id, unipile_account_id, status, weekly_invite_count, weekly_window_started_at, warmup_started_at) ` +
        `VALUES ('${tenantId}','acc_flow20_${suffix}','active', 30, NOW() - INTERVAL '1 day', NOW() - INTERVAL '60 days');`,
    );

    // 20b: capacity — remaining invites this week for the connected account.
    owner.identity.jwt = await owner.identity.mintFreshJWT();
    const cRes = await owner.get('/api/v1/linkedin/capacity');
    expect(cRes.status).toBe(200);
    const c = (await cRes.json()) as Capacity;
    expect(c.connected).toBe(true);
    expect(c.weekly_cap).toBe(100);
    expect(c.weekly_used).toBe(30);
    expect(c.weekly_remaining).toBe(70);

    // 20c: cross-tenant metrics access → 404 (no existence leak).
    await other.get('/api/v1/settings'); // bootstrap
    other.identity.jwt = await other.identity.mintFreshJWT();
    const xRes = await other.get(`/api/v1/campaigns/${campId}/linkedin-metrics`);
    expect(xRes.status).toBe(404);

    await captureArtefact(testInfo, { tenantId });
  } finally {
    // linkedin_events reference campaign_leads (no cascade) — clear them
    // before cleanupTenant deletes the leads; persons are canonical
    // (not tenant-scoped) so they outlive /account and are dropped by
    // their seed marker after the leads are gone.
    if (campId) {
      exec(
        `DELETE FROM linkedin_events WHERE campaign_lead_id IN ` +
          `(SELECT id FROM campaign_leads WHERE campaign_id='${campId}');`,
      );
    }
    if (tenantId) exec(`DELETE FROM linkedin_accounts WHERE tenant_id='${tenantId}';`);
    await cleanupTenant(owner);
    exec(`DELETE FROM persons WHERE normalized_name LIKE 'flow20 %${suffix}';`);
    await cleanupTenant(other);
  }
});
