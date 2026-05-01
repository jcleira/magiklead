"use client";

// Typed wrappers for the /api/v1/admin/* backend endpoints (plan §T14).
// Exposed as a hook so pages pick up Clerk-authenticated fetches via
// useAuth().getToken() without each page having to plumb the token.

import { useAuth } from "@clerk/nextjs";
import { useMemo } from "react";

export interface Conflict {
  id: string;
  canonical_table: string;
  canonical_id: string;
  new_source_record_id: string;
  status: string;
  created_at: string;
  person_canonical_name?: string;
  source_name: string;
  source_record_fields?: Record<string, unknown>;
}

export interface ConflictListResponse {
  results: Conflict[];
  count: number;
}

export interface PersonDetail {
  person: {
    id: string;
    canonical_name: string;
    first_name: string | null;
    last_name: string | null;
    normalized_name: string;
    created_at: string;
    updated_at: string;
  };
  aliases: Array<{ id: string; alias: string; alias_type: string }>;
  identifiers: Array<{
    id: string;
    identifier_type: string;
    identifier_value: string;
    is_primary: boolean | null;
  }>;
  emails: Array<{
    id: string;
    email: string;
    verified_at: string | null;
    verification_method: string | null;
    bounce_count: number;
    is_catchall: boolean | null;
    created_at: string | null;
  }>;
  phones: Array<{
    id: string;
    phone: string;
    verified_at: string | null;
    created_at: string | null;
  }>;
  social_profiles: Array<{
    id: string;
    platform: string;
    handle: string | null;
    url: string | null;
    created_at: string | null;
  }>;
  employments: Array<{
    id: string;
    title: string | null;
    start_date: string | null;
    end_date: string | null;
    is_current: boolean | null;
    organization_id: string;
    organization_name: string;
    primary_domain: string | null;
    created_at: string | null;
  }>;
  evidence: Array<{
    id: string;
    confidence: number;
    conflict_flag: boolean | null;
    created_at: string | null;
    source_record_id: string;
    external_id: string | null;
    fields: Record<string, unknown> | null;
    ingested_at: string | null;
    source_id: string;
    source_name: string;
    source_type: string;
  }>;
}

export interface MergeRequest {
  surviving_id: string;
  merged_id: string;
  reason?: string;
}

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
const JWT_TEMPLATE = "magiklead-backend";

export function useAdminApi() {
  const { getToken, isLoaded, isSignedIn } = useAuth();

  return useMemo(() => {
    async function adminFetch<T>(
      path: string,
      options: RequestInit = {}
    ): Promise<T> {
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
          ...((options.headers as Record<string, string>) || {}),
        },
      });
      if (!res.ok) {
        const body = await res
          .json()
          .catch(() => ({ message: "Unknown error" }));
        throw new Error(body.message || `Admin API error: ${res.status}`);
      }
      return res.json() as Promise<T>;
    }

    return {
      listConflicts(status: string, limit = 50, offset = 0) {
        const qs = new URLSearchParams({
          limit: String(limit),
          offset: String(offset),
        });
        if (status) qs.set("status", status);
        return adminFetch<ConflictListResponse>(
          `/api/v1/admin/conflicts?${qs}`
        );
      },
      rejectConflict(id: string) {
        return adminFetch<{ status: string }>(
          `/api/v1/admin/conflicts/${id}/reject`,
          { method: "POST" }
        );
      },
      mergeConflict(id: string, body: MergeRequest) {
        return adminFetch<{
          status: string;
          surviving_id: string;
          merged_id: string;
        }>(`/api/v1/admin/conflicts/${id}/merge`, {
          method: "POST",
          body: JSON.stringify(body),
        });
      },
      getPerson(id: string) {
        return adminFetch<PersonDetail>(`/api/v1/admin/persons/${id}`);
      },
      deletePerson(id: string) {
        return adminFetch<{ status: string }>(
          `/api/v1/admin/persons/${id}/delete`,
          { method: "POST" }
        );
      },
    };
  }, [getToken, isLoaded, isSignedIn]);
}
