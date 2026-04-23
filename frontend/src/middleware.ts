import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const protectedPaths = [
  "/dashboard",
  "/campaigns",
  "/leads",
  "/settings",
  "/onboarding",
];

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // Check if this is a protected route
  const isProtected = protectedPaths.some((p) => pathname.startsWith(p));
  if (!isProtected) {
    return NextResponse.next();
  }

  // TODO: When Clerk is configured, use clerkMiddleware() instead.
  // For MVP without Clerk keys, allow all access for development.
  // In production, replace this file with:
  //
  // import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
  // const isProtectedRoute = createRouteMatcher(["/dashboard(.*)", ...]);
  // export default clerkMiddleware(async (auth, req) => {
  //   if (isProtectedRoute(req)) await auth.protect();
  // });

  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!.*\\..*|_next).*)", "/", "/(api|trpc)(.*)"],
};
