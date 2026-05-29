// Flow 7: save N unique leads → quota counter advances by exactly
// N; re-saving the same person returns 201 but does NOT bump the
// counter (idempotent quota enforcement).

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryRows, queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

test('flow 7: quota counter increments by unique persons only', async ({}, testInfo) => {
  const client = await newClient('flow7');
  try {
    await client.get('/api/v1/settings'); // bootstrap

    // Pick 5 distinct fixture person_ids with verified emails.
    const rows = queryRows(
      `SELECT DISTINCT ON (p.id) p.id FROM persons p JOIN emails e ON e.person_id=p.id WHERE e.verified_at IS NOT NULL LIMIT 5;`,
    );
    expect(rows).toHaveLength(5);

    for (const [pid] of rows) {
      const res = await client.post('/api/v1/tenant_leads', { person_id: pid });
      expect(res.status).toBe(201);
    }

    const tenantId = resolveTenantID(client.identity.userId);
    const used = parseInt(
      queryScalar(`SELECT leads_used FROM subscriptions WHERE tenant_id='${tenantId}';`),
      10,
    );
    expect(used).toBe(5);

    // Re-save the first one — quota must not double-count.
    const dupRes = await client.post('/api/v1/tenant_leads', { person_id: rows[0][0] });
    expect(dupRes.status).toBe(201);
    const usedAfterDup = parseInt(
      queryScalar(`SELECT leads_used FROM subscriptions WHERE tenant_id='${tenantId}';`),
      10,
    );
    expect(usedAfterDup).toBe(5);

    await captureArtefact(testInfo, { tenantId });
  } finally {
    await cleanupTenant(client);
  }
});
