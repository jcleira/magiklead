// Flow 2: lazy bootstrap. A Clerk user that has NEVER hit a
// protected endpoint must not have a tenant row; the moment any
// protected endpoint fires, EnsureTenant middleware materialises
// the tenant + free subscription on first request.

import { test, expect } from '@playwright/test';
import { mintIdentity, deleteIdentity } from '../helpers/clerk.js';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

test('flow 2: lazy bootstrap on first protected request', async ({}, testInfo) => {
  // Step 1: mint a Clerk user but DO NOT hit the api.
  const idleIdentity = await mintIdentity('flow2-idle');
  try {
    const idleTenantCount = queryScalar(
      `SELECT COUNT(*) FROM users WHERE clerk_id='${idleIdentity.userId}';`,
    );
    expect(idleTenantCount, 'idle user must not have a user row yet').toBe('0');
  } finally {
    await deleteIdentity(idleIdentity.userId);
  }

  // Step 2: a fresh user that hits /settings once should now have
  // tenant + free subscription rows.
  const client = await newClient('flow2-active');
  try {
    const res = await client.get('/api/v1/settings');
    expect(res.status).toBe(200);

    const tenantName = queryScalar(
      `SELECT t.name FROM users u JOIN user_tenants ut ON ut.user_id=u.id JOIN tenants t ON t.id=ut.tenant_id WHERE u.clerk_id='${client.identity.userId}';`,
    );
    expect(tenantName).toContain("'s Workspace");

    const plan = queryScalar(
      `SELECT s.plan FROM users u JOIN user_tenants ut ON ut.user_id=u.id JOIN subscriptions s ON s.tenant_id=ut.tenant_id WHERE u.clerk_id='${client.identity.userId}';`,
    );
    expect(plan).toBe('free');

    await captureArtefact(testInfo, { tenantId: resolveTenantID(client.identity.userId) });
  } finally {
    await cleanupTenant(client);
  }
});
