"use client";

import { useAuth } from "@clerk/nextjs";
import { useCallback } from "react";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

// The backend expects a Clerk session JWT minted from the
// "magiklead-backend" template (see backend/.env.example). That
// template carries the primary email claim used to gate admin routes
// without a DB round-trip.
const JWT_TEMPLATE = "magiklead-backend";

/**
 * Hook that provides an authenticated API fetch function.
 * Uses Clerk's getToken() to mint a signed JWT per request.
 */
export function useApi() {
  const { getToken, isLoaded, isSignedIn } = useAuth();

  const apiFetch = useCallback(
    async <T>(path: string, options?: RequestInit): Promise<T> => {
      if (!isLoaded || !isSignedIn) {
        throw new Error("Not authenticated");
      }
      const token = await getToken({ template: JWT_TEMPLATE });
      if (!token) {
        throw new Error("Not authenticated");
      }

      const res = await fetch(`${API_URL}${path}`, {
        ...options,
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
          ...((options?.headers as Record<string, string>) || {}),
        },
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({
          message: "Unknown error",
        }));
        throw new Error(body.message || `API error: ${res.status}`);
      }

      return res.json();
    },
    [getToken, isLoaded, isSignedIn]
  );

  return { apiFetch };
}
