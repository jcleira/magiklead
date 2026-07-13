// Flow 17: LinkedIn prospects surface in /leads/search.
//
// A LinkedIn-sourced prospect (org + person + current employment +
// linkedin_url identifier, NO email) is seeded into the canonical, then
// POST /leads/search returns it with its linkedin_url and no email —
// proving the handler attaches the profile link and that with_email is
// not required (issue #2, AC #6). The write-through itself is covered by
// the Go integration tests; this asserts the HTTP surface.

import { test, expect } from '@playwright/test';
import crypto from 'node:crypto';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar, exec } from '../helpers/db.js';

test('flow 17: /leads/search returns a LinkedIn prospect with linkedin_url, no email', async () => {
  const client = await newClient('flow17');
  const suffix = crypto.randomUUID().slice(0, 8);
  const title = `Flow17 Chief Test Officer ${suffix}`;
  const url = `https://www.linkedin.com/in/flow17-${suffix}`;
  let personId = '';
  let orgId = '';
  try {
    await client.get('/api/v1/settings'); // lazy-bootstrap the tenant

    // Seed a LinkedIn prospect directly into the canonical (the AFK
    // substitute for a live RapidAPI fetch — the write-through is unit
    // tested; here we only need a row to search for).
    orgId = queryScalar(
      `INSERT INTO organizations (canonical_name, primary_domain) VALUES ('Flow17 Co ${suffix}','flow17-${suffix}.test') RETURNING id;`,
    );
    personId = queryScalar(
      `INSERT INTO persons (canonical_name, first_name, last_name, normalized_name, has_email) VALUES ('Flo Seventeen ${suffix}','Flo','Seventeen','flo seventeen ${suffix}', false) RETURNING id;`,
    );
    exec(
      `INSERT INTO employments (person_id, organization_id, title, is_current) VALUES ('${personId}','${orgId}','${title}', true);`,
    );
    exec(
      `INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary) VALUES ('${personId}','linkedin_url','${url}', true);`,
    );

    // with_email omitted (defaults false) — LinkedIn prospects carry none.
    const res = await client.post('/api/v1/leads/search', { titles: [title], limit: 25 });
    expect(res.status).toBe(200);
    const body = (await res.json()) as {
      results: Array<{ person_id: string; name: string; linkedin_url?: string; email?: string }>;
    };

    const hit = body.results.find((r) => r.person_id === personId);
    expect(hit, 'seeded LinkedIn prospect should be in the results').toBeTruthy();
    expect(hit?.linkedin_url).toBe(url);
    expect(hit?.email ?? null).toBeNull();
  } finally {
    if (personId) exec(`DELETE FROM persons WHERE id='${personId}';`); // cascades identifier + employment
    if (orgId) exec(`DELETE FROM organizations WHERE id='${orgId}';`);
    await cleanupTenant(client);
  }
});
