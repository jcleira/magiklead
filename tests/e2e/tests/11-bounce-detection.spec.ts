// Flow 11: bounce detection (hard + soft + threshold). AFK
// substitute: poller-classifier + worker-integration suites cover
// the DSN parsing + threshold logic. Real bounce → bounced status
// lights up with E2E_REAL_GMAIL=1.

import { test, expect } from '@playwright/test';
import { goTest } from '../helpers/gotest.js';
import { captureArtefact } from '../helpers/artefact.js';

test('flow 11: poller bounce classification + worker bounce-recorded paths', async ({}, testInfo) => {
  test.setTimeout(120_000);

  const pollerOut = goTest(['-v', '-count=1', '-run', 'TestTick_.*Bounce', './internal/gmail/poller/...']);
  expect(pollerOut, pollerOut).toContain('PASS');
  expect(pollerOut, pollerOut).not.toContain('--- FAIL');

  const workerOut = goTest([
    '-v',
    '-count=1',
    '-tags=integration',
    '-run',
    'TestPollOnce_.*Bounce',
    './internal/worker/...',
  ]);
  expect(workerOut, workerOut).toContain('PASS');
  expect(workerOut, workerOut).not.toContain('--- FAIL');

  await captureArtefact(testInfo, { tenantId: null });
});

test('flow 11 (real creds): real bounce against a known-bad address', async () => {
  test.skip(
    process.env.E2E_REAL_GMAIL !== '1',
    'requires E2E_REAL_GMAIL=1 + a deliberately bad recipient',
  );
  // Real-creds path lands in #15 — placeholder until then.
});
