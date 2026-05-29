// Flow 13: per-campaign metrics endpoint accuracy + tenant
// isolation. Seeds N campaign_leads, asserts the JSON contract,
// then proves a cross-tenant request returns 404 (no existence
// leak).

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

interface MetricsPayload {
  leads_total: number;
  sent_total: number;
  sent_by_step: Array<{ step_order: number; count: number }>;
  replied_total: number;
  bounced_total: number;
  unsubscribed_total: number;
}

test('flow 13: /campaigns/{id}/metrics shape + tenant isolation', async ({}, testInfo) => {
  test.setTimeout(120_000);
  const owner = await newClient('flow13owner');
  const other = await newClient('flow13other');
  try {
    // Set up a campaign on the owner.
    await owner.post('/api/v1/websites/analyze', { url: 'https://magikshot.com' });
    owner.identity.jwt = await owner.identity.mintFreshJWT();
    const plays = (await (await owner.post('/api/v1/plays/generate', {})).json()) as Array<{
      id: string;
    }>;
    owner.identity.jwt = await owner.identity.mintFreshJWT();
    const camp = (await (
      await owner.post('/api/v1/campaigns', { play_id: plays[0].id, name: 'E2E flow 13' })
    ).json()) as { id: string };

    // Seed 2 campaign_leads.
    const tenantId = resolveTenantID(owner.identity.userId);
    const pid1 = queryScalar(`SELECT id FROM persons LIMIT 1 OFFSET 10;`);
    const pid2 = queryScalar(`SELECT id FROM persons LIMIT 1 OFFSET 11;`);
    owner.identity.jwt = await owner.identity.mintFreshJWT();
    await owner.post('/api/v1/tenant_leads', { person_id: pid1 });
    await owner.post('/api/v1/tenant_leads', { person_id: pid2 });
    await owner.post(`/api/v1/campaigns/${camp.id}/leads`, { person_ids: [pid1, pid2] });

    // 13a: metrics shape + counts.
    const metricsRes = await owner.get(`/api/v1/campaigns/${camp.id}/metrics`);
    expect(metricsRes.status).toBe(200);
    const m = (await metricsRes.json()) as MetricsPayload;
    expect(m.leads_total).toBe(2);
    expect(m.sent_total).toBe(0);
    expect(Array.isArray(m.sent_by_step)).toBe(true);
    expect(m.replied_total).toBe(0);
    expect(m.bounced_total).toBe(0);
    expect(m.unsubscribed_total).toBe(0);

    // 13b: cross-tenant access → 404.
    await other.get('/api/v1/settings'); // bootstrap
    const xRes = await other.get(`/api/v1/campaigns/${camp.id}/metrics`);
    expect(xRes.status).toBe(404);

    await captureArtefact(testInfo, { tenantId });
  } finally {
    await cleanupTenant(owner);
    await cleanupTenant(other);
  }
});
