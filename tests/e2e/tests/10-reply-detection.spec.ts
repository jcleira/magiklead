// Flow 10: reply detection. AFK substitute proves the classifier
// + worker code path via the existing Go integration tests
// (TestTick_Reply*, TestPollOnce_ReplyRecorded). Real Gmail
// reply → next-tick halt lights up with E2E_REAL_GMAIL=1.

import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { captureArtefact } from '../helpers/artefact.js';

test('flow 10: poller reply classification + worker reply-recorded path', async ({}, testInfo) => {
  test.setTimeout(120_000);

  // Run the poller-classifier suite.
  const pollerOut = execFileSync(
    'devpods',
    ['exec', 'api', 'go', 'test', '-v', '-count=1', '-run', 'TestTick_Reply', './internal/gmail/poller/...'],
    { encoding: 'utf8' },
  );
  expect(pollerOut, pollerOut).toContain('PASS');
  expect(pollerOut, pollerOut).not.toContain('--- FAIL');

  // Run the worker integration suite (reply-recorded path).
  const workerOut = execFileSync(
    'devpods',
    [
      'exec',
      'api',
      'go',
      'test',
      '-v',
      '-count=1',
      '-tags=integration',
      '-run',
      'TestPollOnce_ReplyRecorded',
      './internal/worker/...',
    ],
    { encoding: 'utf8' },
  );
  expect(workerOut, workerOut).toContain('PASS');
  expect(workerOut, workerOut).not.toContain('--- FAIL');

  await captureArtefact(testInfo, { tenantId: null });
});

test('flow 10 (real creds): real Gmail reply → halt', async () => {
  test.skip(
    process.env.E2E_REAL_GMAIL !== '1',
    'requires E2E_REAL_GMAIL=1 + recipient mailbox refresh tokens',
  );
  // Real-creds path lands in #15 — placeholder until then.
});
