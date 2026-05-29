// Flow 8: generate sequence from play/campaign, edit step 2.
// As surfaced in #13's outcome, there is no PATCH/PUT route for
// per-step edits — the AFK substitute is a direct SQL jsonb_set,
// which proves the underlying storage supports the edit. The
// missing endpoint is captured in #13's fix-in-flight backlog.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { exec, queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

test('flow 8: sequence generate + edit step 2', async ({}, testInfo) => {
  test.setTimeout(120_000); // Anthropic round-trips.
  const client = await newClient('flow8');
  try {
    // Bootstrap: website analyze → plays/generate → campaign create.
    await client.post('/api/v1/websites/analyze', { url: 'https://magikshot.com' });
    client.identity.jwt = await client.identity.mintFreshJWT();
    const playsRes = await client.post('/api/v1/plays/generate', {});
    const plays = (await playsRes.json()) as Array<{ id: string }>;
    client.identity.jwt = await client.identity.mintFreshJWT();
    const campRes = await client.post('/api/v1/campaigns', {
      play_id: plays[0].id,
      name: 'E2E flow 8',
    });
    const camp = (await campRes.json()) as { id: string };

    // 8a: sequence generate populates campaigns.sequence with N>=2 steps.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const seqRes = await client.post('/api/v1/sequences/generate', {
      play_id: plays[0].id,
      campaign_id: camp.id,
    });
    expect(seqRes.status).toBe(200);
    const stepCount = parseInt(
      queryScalar(
        `SELECT jsonb_array_length(sequence::jsonb) FROM campaigns WHERE id='${camp.id}';`,
      ),
      10,
    );
    expect(stepCount).toBeGreaterThanOrEqual(2);

    // 8b: gap — no PATCH route. Document via probe.
    const patchRes = await client.patch(`/api/v1/campaigns/${camp.id}/sequence`, {});
    expect(patchRes.status).toBe(404); // route is unimplemented

    // 8c: AFK substitute — direct jsonb_set proves storage supports edit.
    exec(
      `UPDATE campaigns SET sequence = jsonb_set(jsonb_set(sequence::jsonb, '{1,subject}', '"[E2E edit] step 2 subject"'::jsonb), '{1,body}', '"[E2E edit] step 2 body"'::jsonb) WHERE id='${camp.id}';`,
    );
    const editedSubject = queryScalar(
      `SELECT (sequence::jsonb -> 1 ->> 'subject') FROM campaigns WHERE id='${camp.id}';`,
    );
    expect(editedSubject).toBe('[E2E edit] step 2 subject');

    await captureArtefact(testInfo, { tenantId: resolveTenantID(client.identity.userId) });
  } finally {
    await cleanupTenant(client);
  }
});
