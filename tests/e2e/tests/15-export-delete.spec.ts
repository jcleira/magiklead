// Flow 15: /account/export → zip → spot-check; DELETE /account
// → cascade in DB + Clerk-side user gone.

import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { newClient } from '../helpers/api.js';
import { queryScalar } from '../helpers/db.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

const API_BASE = process.env.E2E_API_URL ?? 'http://api-mvp.magiklead.localhost';

test('flow 15: export zip + DELETE cascade + Clerk user gone', async ({}, testInfo) => {
  test.setTimeout(60_000);
  const client = await newClient('flow15');
  await client.get('/api/v1/settings'); // bootstrap

  // Save 1 lead so the export has something non-trivial.
  const pid = queryScalar(`SELECT id FROM persons LIMIT 1 OFFSET 20;`);
  await client.post('/api/v1/tenant_leads', { person_id: pid });

  const tenantId = resolveTenantID(client.identity.userId);

  // 15a: export.
  const here = dirname(fileURLToPath(import.meta.url));
  const zipPath = join(here, '..', 'artefacts', `flow15-export-${Date.now()}.zip`);
  mkdirSync(dirname(zipPath), { recursive: true });

  const exportRes = await fetch(`${API_BASE}/api/v1/account/export`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${client.identity.jwt}` },
  });
  expect(exportRes.status).toBe(200);
  writeFileSync(zipPath, Buffer.from(await exportRes.arrayBuffer()));

  // Inspect zip via `unzip -l`.
  const ls = execFileSync('unzip', ['-l', zipPath], { encoding: 'utf8' });
  for (const name of ['tenant.json', 'user.json', 'subscription.json', 'tenant_leads.json']) {
    expect(ls).toContain(name);
  }

  // tenant.json should carry the workspace id.
  const tenantJson = execFileSync('unzip', ['-p', zipPath, 'tenant.json'], { encoding: 'utf8' });
  const tenantBody = JSON.parse(tenantJson) as { id: string; name: string };
  expect(tenantBody.id).toBe(tenantId);

  await captureArtefact(testInfo, { tenantId });

  // 15b: DELETE /account cascade.
  const delRes = await client.del('/api/v1/account');
  expect(delRes.status).toBe(204);

  const tenantsLeft = queryScalar(`SELECT COUNT(*) FROM tenants WHERE id='${tenantId}';`);
  const usersLeft = queryScalar(`SELECT COUNT(*) FROM users WHERE clerk_id='${client.identity.userId}';`);
  expect(tenantsLeft).toBe('0');
  expect(usersLeft).toBe('0');

  // 15c: Clerk-side user must be gone (DELETE /account also calls Clerk API).
  const clerkRes = await fetch(
    `https://api.clerk.com/v1/users/${client.identity.userId}`,
    { headers: { Authorization: `Bearer ${process.env.CLERK_SECRET_KEY}` } },
  );
  expect(clerkRes.status).toBe(404);

  // No deleteIdentity in finally — the account-delete already did it.
});
