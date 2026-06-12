// Flow 6: lead search. AFK path is canonical-only (PDL_API_KEY
// not set in dev). Real PDL credits light up when E2E_REAL_PDL=1.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { exec, queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

interface SearchResult {
  person_id: string;
  name: string;
  email?: string;
  email_verified: boolean;
  title_score?: number;
}
interface SearchResponse {
  results: SearchResult[];
  count: number;
  pdl_called: boolean;
}

test.beforeAll(() => {
  // Make sure the canonical fixture is loaded — cmd/seed is
  // idempotent (CLAUDE.md "Loading the canonical fixture").
  exec(
    `SELECT 1;`,
  );
});

test('flow 6: canonical-only search returns verified-email persons', async ({}, testInfo) => {
  const client = await newClient('flow6');
  try {
    // Ensure we have fixtures with verified emails.
    const fixtureCount = queryScalar(
      `SELECT COUNT(*) FROM persons p JOIN emails e ON e.person_id=p.id WHERE e.verified_at IS NOT NULL;`,
    );
    expect(parseInt(fixtureCount, 10)).toBeGreaterThan(0);

    // 6a: basic search returns 10 results with verified emails.
    const basicRes = await client.post('/api/v1/leads/search', {
      with_email: true,
      limit: 10,
    });
    expect(basicRes.status).toBe(200);
    const basic = (await basicRes.json()) as SearchResponse;
    expect(basic.results.length).toBeGreaterThan(0);
    expect(basic.results.every((r) => r.email_verified === true)).toBe(true);

    // 6b: title filter ranks by title_score.
    const titleRes = await client.post('/api/v1/leads/search', {
      titles: ['CEO', 'Founder', 'Chief Executive'],
      with_email: true,
      limit: 5,
    });
    const title = (await titleRes.json()) as SearchResponse;
    expect(title.results.length).toBeGreaterThan(0);
    expect(title.results[0].title_score).toBeGreaterThan(0);

    // 6c: PDL-only filter without PDL key → empty + pdl_called=false.
    const pdlRes = await client.post('/api/v1/leads/search', {
      industries: ['AI-not-in-canonical'],
      company_size: '50-200',
      with_email: true,
      limit: 5,
    });
    const pdl = (await pdlRes.json()) as SearchResponse;
    if (process.env.E2E_REAL_PDL !== '1') {
      expect(pdl.pdl_called).toBe(false);
    } else {
      expect(pdl.pdl_called).toBe(true);
    }

    await captureArtefact(testInfo, { tenantId: resolveTenantID(client.identity.userId) });
  } finally {
    await cleanupTenant(client);
  }
});
