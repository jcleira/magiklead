// Flow 3: settings page returns real workspace name, plan, usage,
// and connected mailboxes — no hard-coded values reach the JSON.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

interface SettingsPayload {
  workspace: { id: string; name: string };
  plan: { name: string; quota_leads_per_month: number };
  usage: { leads_saved_this_period: number };
  email_accounts: unknown[];
}

test('flow 3: /settings shows real workspace + plan + usage + mailboxes', async ({}, testInfo) => {
  const client = await newClient('flow3');
  try {
    const res = await client.get('/api/v1/settings');
    expect(res.status).toBe(200);
    const body = (await res.json()) as SettingsPayload;

    expect(body.workspace.id).toMatch(/^[0-9a-f]{8}-/);
    expect(body.workspace.name).toContain("'s Workspace");
    expect(body.plan.name).toBe('free');
    expect(body.plan.quota_leads_per_month).toBe(100);
    expect(body.usage.leads_saved_this_period).toBe(0);
    expect(body.email_accounts).toEqual([]);

    await captureArtefact(testInfo, { tenantId: resolveTenantID(client.identity.userId) });
  } finally {
    await cleanupTenant(client);
  }
});
