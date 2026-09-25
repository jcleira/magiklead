// Browser sign-in for UI flows. Most specs drive the API; a UI flow
// signs the test user into the real frontend so it can click through
// the page the founder uses.
import type { Page } from '@playwright/test';
import { clerk } from '@clerk/testing/playwright';
import type { ClerkIdentity } from './clerk.js';

// signInAs opens the app's public home page (it loads Clerk) and signs
// the identity in with a sign-in token (ticket strategy), so no password
// or email code is needed. The page then carries a Clerk session for
// every protected route.
export async function signInAs(page: Page, identity: ClerkIdentity): Promise<void> {
  if (!process.env.CLERK_TESTING_TOKEN) {
    throw new Error(
      'CLERK_PUBLISHABLE_KEY is not set, so global-setup fetched no Clerk testing token. ' +
        'UI flows need it — see tests/e2e/.env.e2e.example.',
    );
  }
  await page.goto('/');
  await clerk.signIn({ page, emailAddress: identity.email });
}
