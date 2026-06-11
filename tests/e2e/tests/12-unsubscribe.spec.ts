// Flow 12: RFC 8058 one-click unsubscribe.
// We mint the same JWT shape the worker writes (HS256 over
// UNSUBSCRIBE_SIGNING_SECRET) and POST to the public endpoint.
// Verifies the shared-secret invariant + tampered/missing paths.

import { test, expect } from '@playwright/test';
import { createHmac } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { exec, queryScalar } from '../helpers/db.js';
import { captureArtefact } from '../helpers/artefact.js';

function readDevpodSecret(key: string): string {
  // In CI the secret is in the environment (api + worker share the
  // same value); locally it lives in the devpod's injected env file.
  const fromEnv = process.env[key];
  if (fromEnv) return fromEnv;
  const home = process.env.HOME ?? '';
  const out = execFileSync(
    'grep',
    [`^${key}=`, `${home}/.config/devpods/magiklead/.env.backend`],
    { encoding: 'utf8' },
  ).trim();
  return out.slice(key.length + 1);
}

function base64url(input: Buffer | string): string {
  return Buffer.from(input).toString('base64').replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_');
}

function mintUnsubToken(email: string, tenantId: string, secret: string): string {
  const header = base64url('{"alg":"HS256","typ":"JWT"}');
  const payload = base64url(
    JSON.stringify({ email, tenant_id: tenantId, exp: Math.floor(Date.now() / 1000) + 86400 }),
  );
  const signingInput = `${header}.${payload}`;
  const sig = base64url(createHmac('sha256', secret).update(signingInput).digest());
  return `${signingInput}.${sig}`;
}

const API_BASE = process.env.E2E_API_URL ?? 'http://api-mvp.magiklead.localhost';

test('flow 12: public unsubscribe (mint → POST → suppression row)', async ({}, testInfo) => {
  const secret = readDevpodSecret('UNSUBSCRIBE_SIGNING_SECRET');
  const tenantId = queryScalar(`SELECT id FROM tenants LIMIT 1;`); // any tenant works
  const email = `e2e-flow12-${Date.now()}@example.test`;
  const token = mintUnsubToken(email, tenantId, secret);

  // 12a: POST with valid token → 200 HTML confirmation.
  const okRes = await fetch(`${API_BASE}/api/v1/public/unsubscribe?token=${token}`, {
    method: 'POST',
  });
  expect(okRes.status).toBe(200);
  expect(await okRes.text()).toContain(email);

  // 12b: suppression row inserted with reason=list-unsub.
  const reason = queryScalar(`SELECT reason FROM unsubscribes WHERE email='${email}';`);
  expect(reason).toBe('list-unsub');

  // 12c: replay → idempotent (ON CONFLICT).
  const replayRes = await fetch(`${API_BASE}/api/v1/public/unsubscribe?token=${token}`, {
    method: 'POST',
  });
  expect(replayRes.status).toBe(200);
  const count = queryScalar(`SELECT COUNT(*) FROM unsubscribes WHERE email='${email}';`);
  expect(count).toBe('1');

  // 12d: tampered signature → 401.
  const tampered = `${token.slice(0, token.lastIndexOf('.'))}.invalidSig`;
  const tamperedRes = await fetch(`${API_BASE}/api/v1/public/unsubscribe?token=${tampered}`, {
    method: 'POST',
  });
  expect(tamperedRes.status).toBe(401);

  // 12e: missing token → 401.
  const missingRes = await fetch(`${API_BASE}/api/v1/public/unsubscribe`, { method: 'POST' });
  expect(missingRes.status).toBe(401);

  // Cleanup test row.
  exec(`DELETE FROM unsubscribes WHERE email='${email}';`);

  await captureArtefact(testInfo, { tenantId: null });
});
