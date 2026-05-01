import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";

// Next 16 deprecated `middleware.ts` in favor of `proxy.ts`; keeping
// the old filename here as an intermediate step. The codemod
// `npx @next/codemod@canary middleware-to-proxy .` will rename this
// once the rest of the initial-release plan lands.
const isProtectedRoute = createRouteMatcher([
  "/dashboard(.*)",
  "/campaigns(.*)",
  "/leads(.*)",
  "/settings(.*)",
  "/onboarding(.*)",
  // Admin routes live in the (admin) group on the file system but the
  // URL path is top-level (e.g. /conflicts, /persons/[id]).
  "/conflicts(.*)",
  "/persons(.*)",
]);

export default clerkMiddleware(async (auth, req) => {
  if (isProtectedRoute(req)) {
    await auth.protect();
  }
});

export const config = {
  matcher: [
    // Skip Next.js internals and static assets unless referenced in the
    // URL string itself.
    "/((?!_next|[^?]*\\.(?:html?|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)",
    // Always run for API routes.
    "/(api|trpc)(.*)",
  ],
};
