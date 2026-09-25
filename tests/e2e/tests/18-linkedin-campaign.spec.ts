// Flow 18: author a LinkedIn campaign + add saved prospects in the UI
// (issue #3; the add-leads picker, docs/2026-07-13-magikshot-linkedin-smoke).
//
// POST /campaigns with channel='linkedin' and a valid linkedin_sequence
// (a step-0 connection note ≤300 chars — or none, for a plain invite —
// + ≥1 DM step) persists the channel and the sequence; an over-limit
// note is rejected 4xx. Then, in the browser, the user opens the draft,
// reads the note on the page, and adds a saved LinkedIn prospect with
// the "Add saved leads" picker: it lands as a campaign_leads row with
// its person_id and status='queued', and the leads table links to the
// profile. A saved lead without a LinkedIn profile cannot be picked, and
// with no LinkedIn account connected, Start stays closed. No sending
// happens here.
//
// API status codes + direct DB reads are the house pattern (see flows
// 09/17); the picker step drives the real page because it is the one
// the founder uses. The play is seeded directly rather than via the
// Anthropic onboarding round-trip — the LinkedIn rail only needs a row
// to satisfy the campaigns.play_id FK.

import { test, expect } from '@playwright/test';
import crypto from 'node:crypto';
import { newClient, cleanupTenant } from '../helpers/api.js';
import { signInAs } from '../helpers/browser.js';
import { queryScalar, queryJSON, queryRows, exec } from '../helpers/db.js';
import { resolveTenantID } from '../helpers/artefact.js';

interface LinkedinStep {
  step: number;
  delay_days: number;
  body: string;
}

test('flow 18: author a LinkedIn campaign and add saved prospects in the UI', async ({ page }) => {
  const client = await newClient('flow18');
  const suffix = crypto.randomUUID().slice(0, 8);
  const personIds: string[] = [];
  let orgId = '';
  try {
    await client.get('/api/v1/settings'); // lazy-bootstrap the tenant
    const tenantId = resolveTenantID(client.identity.userId);

    // A play to attach the campaign to (FK target only).
    const playId = queryScalar(
      `INSERT INTO plays (tenant_id, name, icp, search_query, channels) ` +
        `VALUES ('${tenantId}','Flow18 Play ${suffix}','{}'::jsonb,'{}'::jsonb, ARRAY['linkedin']::text[]) RETURNING id;`,
    );

    const note = 'Hi {{first_name}}, loved what {{company}} is building — open to connecting?';
    const sequence: LinkedinStep[] = [
      { step: 0, delay_days: 0, body: note },
      { step: 1, delay_days: 2, body: 'Thanks for connecting, {{first_name}}! Curious how {{company}} handles outreach.' },
    ];

    // Over-limit step-0 note (2 steps, so the count check passes and the
    // 300-char note check is what trips) → 400.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const tooLong = await client.post('/api/v1/campaigns', {
      play_id: playId,
      name: `Flow18 LI overflow ${suffix}`,
      channel: 'linkedin',
      linkedin_sequence: [{ step: 0, delay_days: 0, body: 'x'.repeat(301) }, sequence[1]],
    });
    expect(tooLong.status, 'over-limit connection note must be rejected').toBe(400);

    // An empty note is a plain invite (a free LinkedIn account sends about
    // 5 invites with a note per month) → 201.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const noNote = await client.post('/api/v1/campaigns', {
      play_id: playId,
      name: `Flow18 LI no note ${suffix}`,
      channel: 'linkedin',
      linkedin_sequence: [{ step: 0, delay_days: 0, body: '' }, sequence[1]],
    });
    expect(noNote.status, 'a campaign without a connection note must be accepted').toBe(201);

    // Happy path → 201.
    client.identity.jwt = await client.identity.mintFreshJWT();
    const createRes = await client.post('/api/v1/campaigns', {
      play_id: playId,
      name: `Flow18 LI ${suffix}`,
      channel: 'linkedin',
      linkedin_sequence: sequence,
    });
    const createBody = await createRes.text();
    expect(createRes.status, createBody).toBe(201);
    const camp = JSON.parse(createBody) as { id: string };

    // DB: channel persisted as 'linkedin'.
    expect(queryScalar(`SELECT channel FROM campaigns WHERE id='${camp.id}';`)).toBe('linkedin');

    // DB: linkedin_sequence stored with a step-0 note + ≥1 DM step.
    const stored = queryJSON<LinkedinStep[]>(
      `SELECT linkedin_sequence FROM campaigns WHERE id='${camp.id}';`,
    );
    expect(Array.isArray(stored)).toBe(true);
    expect(stored.length).toBeGreaterThanOrEqual(2);
    expect(stored[0].step).toBe(0);
    expect(stored[0].body).toBe(note);
    expect(stored.some((s) => s.step >= 1), 'sequence must carry at least one DM step').toBe(true);

    // Seed a LinkedIn prospect (no email) and a person with no LinkedIn
    // profile, both at one company, and save both to the tenant.
    orgId = queryScalar(
      `INSERT INTO organizations (canonical_name, primary_domain) VALUES ('Flow18 Co ${suffix}','flow18-${suffix}.test') RETURNING id;`,
    );
    const seedPerson = (first: string, last: string): string => {
      const id = queryScalar(
        `INSERT INTO persons (canonical_name, first_name, last_name, normalized_name, has_email) ` +
          `VALUES ('${first} ${last} ${suffix}','${first}','${last}','${first.toLowerCase()} ${last.toLowerCase()} ${suffix}', false) RETURNING id;`,
      );
      exec(
        `INSERT INTO employments (person_id, organization_id, title, is_current) VALUES ('${id}','${orgId}','Managing Partner', true);`,
      );
      personIds.push(id);
      return id;
    };
    const personId = seedPerson('Lin', 'Eighteen');
    const noProfileId = seedPerson('Nolan', 'Profile');
    const profileURL = `https://www.linkedin.com/in/flow18-${suffix}`;
    exec(
      `INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary) ` +
        `VALUES ('${personId}','linkedin_url','${profileURL}', true);`,
    );
    for (const id of [personId, noProfileId]) {
      client.identity.jwt = await client.identity.mintFreshJWT();
      const saveRes = await client.post('/api/v1/tenant_leads', { person_id: id });
      expect(saveRes.ok, `save lead → ${saveRes.status}`).toBeTruthy();
    }

    // UI: open the draft as the user.
    await signInAs(page, client.identity);
    await page.goto(`/campaigns/${camp.id}`);
    await expect(page.getByRole('heading', { name: 'Set up your LinkedIn campaign' })).toBeVisible();
    // The copy is on the page for review, and the email setup is not.
    await expect(page.getByText(note)).toBeVisible();
    await expect(page.getByText('Sent when the invite is accepted')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Discover Leads' })).toHaveCount(0);
    // No lead yet: Start stays closed.
    await expect(page.getByText('Add at least one lead with a LinkedIn profile first.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Start campaign' })).toHaveCount(0);

    // Add the saved prospect with the picker.
    await page.getByRole('button', { name: 'Add saved leads' }).click();
    const dialog = page.getByRole('dialog', { name: 'Add saved leads' });
    await expect(dialog).toBeVisible();
    const pick = dialog.getByRole('checkbox', { name: `Select Lin Eighteen ${suffix}` });
    await expect(pick).toBeEnabled();
    await expect(dialog.getByRole('checkbox', { name: `Select Nolan Profile ${suffix}` })).toBeDisabled();
    await expect(dialog.getByText('No LinkedIn profile')).toBeVisible();
    await pick.check();
    await dialog.getByRole('button', { name: 'Add 1 lead' }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText('Added 1 lead.')).toBeVisible();

    // The leads table links to the profile the invite goes to.
    const row = page.getByRole('row', { name: /Lin Eighteen/ });
    await expect(row).toBeVisible();
    await expect(row.getByRole('link', { name: 'Profile ↗' })).toHaveAttribute('href', profileURL);

    // A lead is in, but no LinkedIn account is connected: Start stays
    // closed and points to Settings.
    await expect(page.getByText('Connect your LinkedIn account in Settings first.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Start campaign' })).toHaveCount(0);

    // DB: one campaign_leads row, keyed on person_id, status 'queued'.
    const rows = queryRows(
      `SELECT person_id, status FROM campaign_leads WHERE campaign_id='${camp.id}';`,
    );
    expect(rows.length).toBe(1);
    expect(rows[0][0]).toBe(personId);
    expect(rows[0][1]).toBe('queued');

    // The API keeps the person with no profile out even when asked
    // directly (the picker only hides it).
    client.identity.jwt = await client.identity.mintFreshJWT();
    const direct = await client.post(`/api/v1/campaigns/${camp.id}/leads`, { person_ids: [noProfileId] });
    expect(direct.status).toBe(200);
    expect(await direct.json()).toMatchObject({ added: 0, skipped: 1, skipped_no_linkedin: 1 });
  } finally {
    for (const id of personIds) exec(`DELETE FROM persons WHERE id='${id}';`); // cascades identifiers + jobs
    if (orgId) exec(`DELETE FROM organizations WHERE id='${orgId}';`);
    await cleanupTenant(client);
  }
});
