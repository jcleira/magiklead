"use client";

import { useCallback } from "react";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/**
 * Hook that provides an authenticated API fetch function.
 * In production this will use Clerk's getToken().
 * For MVP, passes a placeholder token.
 */
export function useApi() {
  const apiFetch = useCallback(
    async <T>(path: string, options?: RequestInit): Promise<T> => {
      // TODO(T16): Use Clerk's useAuth().getToken() for real JWT
      const token = "dev-token";

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
    []
  );

  return { apiFetch };
}
