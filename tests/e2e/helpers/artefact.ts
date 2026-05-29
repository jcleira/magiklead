// captureArtefact persists the per-spec evidence the operator
// browses in CI: a full-page screenshot of the current viewport
// plus a JSON snapshot of the tables that matter for the flow
// (tenants, subscriptions, campaign_leads, email_events,
// unsubscribes), scoped to the test's own tenant when one exists.
//
// Specs that drive the UI pass `page`; pure API specs pass
// `page: null` and only the DB snapshot is written. Either way
// the artefact ends up under tests/e2e/artefacts/<spec>/<test>.

import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import type { Page, TestInfo } from '@playwright/test';
import { queryJSON, queryScalar } from './db.js';

export interface ArtefactOptions {
  // The tenant scope for the DB snapshot. Pass null to skip the
  // tenant-scoped queries (only the screenshot is written).
  tenantId?: string | null;
  // The Page being inspected; pass null for pure API specs.
  page?: Page | null;
}

function safeName(name: string): string {
  return name.replace(/[^a-z0-9-]+/gi, '_').replace(/^_+|_+$/g, '');
}

export async function captureArtefact(
  testInfo: TestInfo,
  opts: ArtefactOptions = {},
): Promise<void> {
  const root = join(testInfo.project.testDir, '..', 'artefacts');
  const dir = join(root, safeName(testInfo.titlePath.slice(0, -1).join('-') || 'root'));
  const stem = safeName(testInfo.title);

  // Screenshot first — if the page is closed the catch keeps the
  // DB snapshot from being skipped.
  if (opts.page) {
    try {
      mkdirSync(dir, { recursive: true });
      await opts.page.screenshot({ path: join(dir, `${stem}.png`), fullPage: true });
    } catch (err) {
      // Best-effort: a failed screenshot must not mask the test
      // result. Note it in the JSON snapshot's "errors" field.
      writeFileSync(join(dir, `${stem}.screenshot.err`), String(err));
    }
  }

  // DB snapshot — counts + sample rows for the tenant under test.
  // When tenantId is null we still capture the global counts so
  // operator can spot order-of-magnitude regressions.
  const snapshot: Record<string, unknown> = {
    captured_at: new Date().toISOString(),
    test: testInfo.titlePath.join(' › '),
    tenant_id: opts.tenantId ?? null,
  };

  if (opts.tenantId) {
    snapshot.tenant = queryJSON(
      `SELECT row_to_json(t) FROM (SELECT id, name, created_at FROM tenants WHERE id='${opts.tenantId}') t;`,
    );
    snapshot.subscription = queryJSON(
      `SELECT COALESCE((SELECT row_to_json(s) FROM (SELECT plan, leads_limit, leads_used FROM subscriptions WHERE tenant_id='${opts.tenantId}') s), 'null'::json);`,
    );
    snapshot.campaign_leads = queryJSON(
      `SELECT COALESCE(json_agg(row_to_json(r)), '[]'::json) FROM (SELECT cl.id, cl.status, cl.current_step FROM campaign_leads cl JOIN campaigns c ON c.id=cl.campaign_id WHERE c.tenant_id='${opts.tenantId}') r;`,
    );
    snapshot.email_events = queryJSON(
      `SELECT COALESCE(json_agg(row_to_json(r)), '[]'::json) FROM (SELECT ee.event_type, ee.step FROM email_events ee JOIN campaign_leads cl ON cl.id=ee.campaign_lead_id JOIN campaigns c ON c.id=cl.campaign_id WHERE c.tenant_id='${opts.tenantId}') r;`,
    );
    snapshot.unsubscribes = queryJSON(
      `SELECT COALESCE(json_agg(row_to_json(r)), '[]'::json) FROM (SELECT email, reason FROM unsubscribes WHERE tenant_id='${opts.tenantId}') r;`,
    );
  } else {
    snapshot.global_counts = queryJSON(
      `SELECT row_to_json(c) FROM (SELECT (SELECT COUNT(*) FROM tenants) AS tenants, (SELECT COUNT(*) FROM subscriptions) AS subscriptions, (SELECT COUNT(*) FROM campaign_leads) AS campaign_leads, (SELECT COUNT(*) FROM email_events) AS email_events) c;`,
    );
  }

  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, `${stem}.json`), JSON.stringify(snapshot, null, 2));

  // Attach to the Playwright report so artefacts surface in the
  // HTML viewer even without going to disk.
  await testInfo.attach(`db-snapshot-${stem}.json`, {
    body: JSON.stringify(snapshot, null, 2),
    contentType: 'application/json',
  });
}

// resolveTenantID looks up the tenant for a Clerk user. Specs that
// have already bootstrapped the tenant (e.g. via `bootstrapTenant`)
// use this to grab the UUID for captureArtefact.
export function resolveTenantID(clerkUserID: string): string {
  const sql = `SELECT t.id FROM users u JOIN user_tenants ut ON ut.user_id=u.id JOIN tenants t ON t.id=ut.tenant_id WHERE u.clerk_id='${clerkUserID}';`;
  return queryScalar(sql);
}
