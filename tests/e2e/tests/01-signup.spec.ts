// Flow 1 from #13's Outcome table: fresh Clerk sign-up surrogate.
// We mint a Clerk user via the Backend API (the hosted-UI walk
// requires real human interaction with the Clerk dialog and is
// owed to #15), then assert every endpoint a freshly-loaded
// dashboard would hit returns 200. This proves the auth chain
// (Clerk JWT → middleware → lazy-bootstrap → handler) works
// end-to-end on a brand-new identity.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

test('flow 1: fresh sign-up → every dashboard endpoint serves 200', async ({}, testInfo) => {
  const client = await newClient('flow1');
  try {
    const paths = [
      '/api/v1/settings',
      '/api/v1/campaigns',
      '/api/v1/plays',
      '/api/v1/leads',
      '/api/v1/tenant_leads',
      '/api/v1/email-accounts',
      '/api/v1/gmail/accounts',
      '/api/v1/billing/subscription',
    ];
    for (const path of paths) {
      const res = await client.get(path);
      expect(res.status, `${path} should be 200, got ${res.status}`).toBe(200);
    }
    await captureArtefact(testInfo, { tenantId: resolveTenantID(client.identity.userId) });
  } finally {
    await cleanupTenant(client);
  }
});
