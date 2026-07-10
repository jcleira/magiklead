// Flow 18: author a LinkedIn campaign + queue saved prospects (issue #3).
//
// POST /campaigns with channel='linkedin' and a valid linkedin_sequence
// (a step-0 connection note ≤300 chars + ≥1 DM step) persists the
// channel and the sequence; an over-limit note is rejected 4xx. Adding
// a saved LinkedIn prospect (no email) then queues a campaign_leads row
// with its person_id and status='queued'. No sending happens here.
//
// Assertions are API status codes + direct DB reads (the house pattern;
// see flows 09/17). The play is seeded directly rather than via the
// Anthropic onboarding round-trip — the LinkedIn rail only needs a row
// to satisfy the campaigns.play_id FK.

import { test, expect } from '@playwright/test';
import crypto from 'node:crypto';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar, queryJSON, queryRows, exec } from '../helpers/db.js';
import { resolveTenantID } from '../helpers/artefact.js';

interface LinkedinStep {
  step: number;
  delay_days: number;
  body: string;
}

test('flow 18: author a LinkedIn campaign and queue saved prospects', async () => {
  const client = await newClient('flow18');
  const suffix = crypto.randomUUID().slice(0, 8);
  let personId = '';
  let orgId = '';
  try {
    await client.get('/api/v1/settings'); // lazy-bootstrap the tenant
    const tenantId = resolveTenantID(client.identity.userId);

    // A play to attach the campaign to (FK target only).
    const playId = queryScalar(
      `INSERT INTO plays (tenant_id, name, icp, search_query, channels) ` +
        `VALUES ('${tenantId}','Flow18 Play ${suffix}','{}'::jsonb,'{}'::jsonb, ARRAY['linkedin']::text[]) RETURNING id;`,
    );

    const note = 'Hi {{first_name}}, loved what {{company}} is building — open to connecting?';
    const sequence: LinkedinStep[] = [
      { step: 0, delay_days: 0, body: note },
      { step: 1, delay_days: 2, body: 'Thanks for connecting, {{first_name}}! Curious how {{company}} handles outreach.' },
    ];

    // Over-limit step-0 note (2 steps, so the count check passes and the
    // 300-char note check is what trips) → 400.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const tooLong = await client.post('/api/v1/campaigns', {
      play_id: playId,
      name: `Flow18 LI overflow ${suffix}`,
      channel: 'linkedin',
      linkedin_sequence: [{ step: 0, delay_days: 0, body: 'x'.repeat(301) }, sequence[1]],
    });
    expect(tooLong.status, 'over-limit connection note must be rejected').toBe(400);

    // Happy path → 201.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const createRes = await client.post('/api/v1/campaigns', {
      play_id: playId,
      name: `Flow18 LI ${suffix}`,
      channel: 'linkedin',
      linkedin_sequence: sequence,
    });
    const createBody = await createRes.text();
    expect(createRes.status, createBody).toBe(201);
    const camp = JSON.parse(createBody) as { id: string };

    // DB: channel persisted as 'linkedin'.
    expect(queryScalar(`SELECT channel FROM campaigns WHERE id='${camp.id}';`)).toBe('linkedin');

    // DB: linkedin_sequence stored with a step-0 note + ≥1 DM step.
    const stored = queryJSON<LinkedinStep[]>(
      `SELECT linkedin_sequence FROM campaigns WHERE id='${camp.id}';`,
    );
    expect(Array.isArray(stored)).toBe(true);
    expect(stored.length).toBeGreaterThanOrEqual(2);
    expect(stored[0].step).toBe(0);
    expect(stored[0].body).toBe(note);
    expect(stored.some((s) => s.step >= 1), 'sequence must carry at least one DM step').toBe(true);

    // Seed a LinkedIn prospect (no email) and save it to the tenant.
    orgId = queryScalar(
      `INSERT INTO organizations (canonical_name, primary_domain) VALUES ('Flow18 Co ${suffix}','flow18-${suffix}.test') RETURNING id;`,
    );
    personId = queryScalar(
      `INSERT INTO persons (canonical_name, first_name, last_name, normalized_name, has_email) ` +
        `VALUES ('Lin Eighteen ${suffix}','Lin','Eighteen','lin eighteen ${suffix}', false) RETURNING id;`,
    );
    exec(
      `INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary) ` +
        `VALUES ('${personId}','linkedin_url','https://www.linkedin.com/in/flow18-${suffix}', true);`,
    );
    client.identity.jwt = await client.identity.mintFreshJWT();
    const saveRes = await client.post('/api/v1/tenant_leads', { person_id: personId });
    expect(saveRes.ok, `save lead → ${saveRes.status}`).toBeTruthy();

    // Add the saved prospect to the LinkedIn campaign → queued.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const addRes = await client.post(`/api/v1/campaigns/${camp.id}/leads`, { person_ids: [personId] });
    expect(addRes.status).toBe(200);
    expect(((await addRes.json()) as { added: number }).added).toBe(1);

    // DB: one campaign_leads row, keyed on person_id, status 'queued'.
    const rows = queryRows(
      `SELECT person_id, status FROM campaign_leads WHERE campaign_id='${camp.id}';`,
    );
    expect(rows.length).toBe(1);
    expect(rows[0][0]).toBe(personId);
    expect(rows[0][1]).toBe('queued');
  } finally {
    if (personId) exec(`DELETE FROM persons WHERE id='${personId}';`); // cascades identifier
    if (orgId) exec(`DELETE FROM organizations WHERE id='${orgId}';`);
    await cleanupTenant(client);
  }
});
