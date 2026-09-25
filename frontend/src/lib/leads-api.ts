"use client";

// Typed wrappers for the customer-facing lead endpoints: canonical
// search (/api/v1/leads/search) and the per-tenant saved-leads store
// (/api/v1/tenant_leads). Plan §T04/§T05.

import { useAuth } from "@clerk/nextjs";
import { useMemo } from "react";

export interface LeadSearchResult {
  person_id: string;
  name: string;
  first_name?: string;
  last_name?: string;
  title?: string;
  location?: string;
  organization_id: string;
  organization_name: string;
  domain?: string;
  // Set for LinkedIn-sourced prospects (issue #2); rendered as a profile
  // link. These prospects carry no email.
  linkedin_url?: string;
  industries?: string[];
  company_size?: string;
  email?: string;
  email_verified: boolean;
  email_is_catchall: boolean;
  title_score?: number;
}

export interface LeadSearchResponse {
  results: LeadSearchResult[];
  count: number;
  pdl_called?: boolean;
}

export interface LeadSearchRequest {
  titles?: string[];
  with_email?: boolean;
  // Only people with a LinkedIn profile — the only people the LinkedIn
  // rail can invite.
  with_linkedin?: boolean;
  limit?: number;
  offset?: number;
  industries?: string[];
  company_size?: string;
  locations?: string[];
  description?: string;
}

export interface SavedLead {
  person_id: string;
  status: string;
  notes?: string;
  added_at: string;
  name?: string;
  first_name?: string;
  last_name?: string;
  title?: string;
  company?: string;
  // The profile the LinkedIn rail invites. Absent: this lead cannot join
  // a LinkedIn campaign.
  linkedin_url?: string;
}

export interface SavedLeadListResponse {
  results: SavedLead[];
  count: number;
}

// POST /api/v1/leads/linkedin-search — a Sales Navigator people search
// run as the tenant's connected LinkedIn account. Send the same filters
// with `cursor` to get the next page.
export interface LinkedInSearchRequest {
  titles?: string[];
  locations?: string[];
  industries?: string[];
  company_size?: string;
  cursor?: string;
}

export interface LinkedInSearchResult {
  person_id: string;
  name: string;
  first_name?: string;
  last_name?: string;
  title?: string;
  company?: string;
  location?: string;
  headline?: string;
  linkedin_url: string;
  // This LinkedIn account already invited the person.
  pending_invitation: boolean;
}

// Which LinkedIn value a typed location or industry matched.
export interface LinkedInMatchedParam {
  query: string;
  id: string;
  title: string;
}

export interface LinkedInSearchResponse {
  results: LinkedInSearchResult[];
  count: number;
  // Results LinkedIn returned without a public profile (not stored).
  hidden: number;
  cursor: string;
  total: number;
  locations: LinkedInMatchedParam[];
  industries: LinkedInMatchedParam[];
  used_today: number;
  daily_cap: number;
}

// The saved-leads list is paged at 200 by the API; the add-leads picker
// needs every saved lead.
const SAVED_PAGE = 200;
const SAVED_MAX_PAGES = 10;

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
const JWT_TEMPLATE = "magiklead-backend";

// LeadsApiError carries the API's error code (for example
// "linkedin_not_connected") so a page can react to it.
export class LeadsApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export function useLeadsApi() {
  const { getToken, isLoaded, isSignedIn } = useAuth();

  return useMemo(() => {
    async function call<T>(path: string, options: RequestInit = {}): Promise<T> {
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
        throw new LeadsApiError(
          res.status,
          body.code || "unknown",
          body.message || `Leads API error: ${res.status}`
        );
      }
      if (res.status === 204) {
        return undefined as T;
      }
      return res.json() as Promise<T>;
    }

    function listSavedLeads(params?: { status?: string; limit?: number; offset?: number }) {
      const qs = new URLSearchParams();
      if (params?.status) qs.set("status", params.status);
      if (params?.limit) qs.set("limit", String(params.limit));
      if (params?.offset) qs.set("offset", String(params.offset));
      const query = qs.toString();
      return call<SavedLeadListResponse>(
        `/api/v1/tenant_leads${query ? `?${query}` : ""}`
      );
    }

    return {
      searchLeads(req: LeadSearchRequest) {
        return call<LeadSearchResponse>("/api/v1/leads/search", {
          method: "POST",
          body: JSON.stringify(req),
        });
      },
      searchLinkedIn(req: LinkedInSearchRequest) {
        return call<LinkedInSearchResponse>("/api/v1/leads/linkedin-search", {
          method: "POST",
          body: JSON.stringify(req),
        });
      },
      saveLead(personId: string, opts?: { status?: string; notes?: string }) {
        return call<SavedLead>("/api/v1/tenant_leads", {
          method: "POST",
          body: JSON.stringify({ person_id: personId, ...opts }),
        });
      },
      listSavedLeads,
      // Every saved lead, page by page (at most SAVED_MAX_PAGES pages).
      async listAllSavedLeads() {
        const all: SavedLead[] = [];
        for (let page = 0; page < SAVED_MAX_PAGES; page++) {
          const res = await listSavedLeads({ limit: SAVED_PAGE, offset: page * SAVED_PAGE });
          all.push(...res.results);
          if (res.results.length < SAVED_PAGE || all.length >= res.count) break;
        }
        return all;
      },
      updateSavedLead(
        personId: string,
        patch: { status?: string; notes?: string }
      ) {
        return call<{ status: string }>(`/api/v1/tenant_leads/${personId}`, {
          method: "PATCH",
          body: JSON.stringify(patch),
        });
      },
      removeSavedLead(personId: string) {
        return call<void>(`/api/v1/tenant_leads/${personId}`, {
          method: "DELETE",
        });
      },
    };
  }, [getToken, isLoaded, isSignedIn]);
}
