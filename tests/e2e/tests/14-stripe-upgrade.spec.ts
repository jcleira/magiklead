// Flow 14: Stripe Checkout up to the API boundary. AFK substitute
// asserts the validation paths + documents the env-var convention
// bug surfaced in #13. Real Stripe Live + promo code lights up
// with E2E_REAL_STRIPE=1.

import { test, expect } from '@playwright/test';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { captureArtefact, resolveTenantID } from '../helpers/artefact.js';

interface SubscriptionPayload {
  plan: string;
  leads_limit: number;
}

test('flow 14: billing routes validation + AFK Stripe boundary', async ({}, testInfo) => {
  const client = await newClient('flow14');
  try {
    await client.get('/api/v1/settings'); // bootstrap

    // 14a: empty body → 400.
    const emptyRes = await client.post('/api/v1/billing/checkout', {});
    expect(emptyRes.status).toBe(400);
    expect(((await emptyRes.json()) as { code: string }).code).toBe('bad_request');

    // 14b: unknown plan → 400.
    const unknownRes = await client.post('/api/v1/billing/checkout', { plan: 'unicorn' });
    expect(unknownRes.status).toBe(400);
    expect(((await unknownRes.json()) as { code: string }).code).toBe('unknown_plan');

    // 14c: bug-flag — 'starter' returns unknown_plan because env
    // vars are STRIPE_PRICE_STARTER_MONTHLY not STRIPE_PRICE_STARTER.
    // The real-Stripe path lights up when E2E_REAL_STRIPE=1 AND
    // the env convention is fixed (see #13 fix-in-flight #6).
    const starterRes = await client.post('/api/v1/billing/checkout', { plan: 'starter' });
    if (process.env.E2E_REAL_STRIPE === '1') {
      expect(starterRes.status).toBe(200);
      const body = (await starterRes.json()) as { url: string };
      expect(body.url).toMatch(/^https:\/\/checkout\.stripe\.com\//);
    } else {
      expect(starterRes.status).toBe(400);
    }

    // 14d: /billing/subscription returns the free-tier row.
    const subRes = await client.get('/api/v1/billing/subscription');
    expect(subRes.status).toBe(200);
    const sub = (await subRes.json()) as SubscriptionPayload;
    expect(sub.plan).toBe('free');
    expect(sub.leads_limit).toBe(100);

    await captureArtefact(testInfo, { tenantId: resolveTenantID(client.identity.userId) });
  } finally {
    await cleanupTenant(client);
  }
});
