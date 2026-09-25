// Global setup: UI flows sign a browser in through Clerk, and Clerk's
// bot protection blocks a scripted sign-in unless the Frontend API
// requests carry a testing token. clerkSetup fetches that token (it
// needs CLERK_SECRET_KEY and CLERK_PUBLISHABLE_KEY). API-only flows need
// none of this, so without a publishable key the setup is skipped and
// only a UI flow fails (with a clear error from helpers/browser.ts).
import { clerkSetup } from '@clerk/testing/playwright';

export default async function globalSetup(): Promise<void> {
  if (!process.env.CLERK_PUBLISHABLE_KEY && !process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY) {
    return;
  }
  await clerkSetup();
}
