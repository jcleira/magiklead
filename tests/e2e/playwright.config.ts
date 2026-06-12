import { defineConfig, devices } from '@playwright/test';
import { config as loadEnv } from 'dotenv';
import { existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const userEnv = join(process.env.HOME ?? '', '.config/devpods/magiklead/.env.e2e');
const localEnv = join(here, '.env.e2e');
if (existsSync(userEnv)) loadEnv({ path: userEnv });
if (existsSync(localEnv)) loadEnv({ path: localEnv, override: true });

export default defineConfig({
  testDir: './tests',
  timeout: 90_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  outputDir: 'test-results',
  use: {
    baseURL: process.env.E2E_FRONTEND_URL ?? 'http://mvp.magiklead.localhost',
    extraHTTPHeaders: {},
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
