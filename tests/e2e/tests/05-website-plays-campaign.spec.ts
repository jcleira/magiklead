// Flow 5: paste website → AI plays → first campaign created.
// Hits real Anthropic for both /websites/analyze and
// /plays/generate; the ANTHROPIC_API_KEY in .env.backend is live.
// The campaign create proves the no-dash hex play id round-trips
// through uuid.Parse correctly.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

test('flow 5: website → plays → campaign', async ({}, testInfo) => {
  test.setTimeout(120_000); // Anthropic round-trips can be slow.
  const client = await newClient('flow5');
  try {
    // 5a: /websites/analyze persists the BusinessProfile.
    const analyzeRes = await client.post('/api/v1/websites/analyze', {
      url: 'https://magikshot.com',
    });
    expect(analyzeRes.status).toBe(200);
    const profile = (await analyzeRes.json()) as Record<string, unknown>;
    expect(profile.company_name).toBeTruthy();
    expect(profile.features).toBeTruthy();

    // 5b: /plays/generate produces ≥1 play with the expected shape.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const playsRes = await client.post('/api/v1/plays/generate', {});
    expect(playsRes.status).toBe(200);
    const plays = (await playsRes.json()) as Array<{ id: string; name: string }>;
    expect(plays.length).toBeGreaterThan(0);
    expect(plays[0].id).toMatch(/^[0-9a-f]{32}$/);

    // 5c: /campaigns POST creates the campaign with status='draft'.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const campRes = await client.post('/api/v1/campaigns', {
      play_id: plays[0].id,
      name: 'E2E flow 5',
    });
    expect(campRes.status).toBe(201);
    const camp = (await campRes.json()) as { id: string; status: string };
    expect(camp.status).toBe('draft');

    // DB confirms.
    const tenantId = resolveTenantID(client.identity.userId);
    const dbStatus = queryScalar(
      `SELECT status FROM campaigns WHERE tenant_id='${tenantId}' LIMIT 1;`,
    );
    expect(dbStatus).toBe('draft');

    await captureArtefact(testInfo, { tenantId });
  } finally {
    await cleanupTenant(client);
  }
});
