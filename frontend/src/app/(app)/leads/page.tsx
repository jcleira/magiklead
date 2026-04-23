"use client";

import { useEffect, useState } from "react";
import { useApi } from "@/hooks/use-api";

interface Lead {
  id: string;
  first_name: string;
  last_name: string;
  title: string | null;
  company: string | null;
  email: string | null;
  linkedin_url: string | null;
  source: string | null;
  created_at: string;
}

export default function LeadsPage() {
  const { apiFetch } = useApi();
  const [leads, setLeads] = useState<Lead[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  // Debounce search
  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), 300);
    return () => clearTimeout(t);
  }, [search]);

  useEffect(() => {
    setLoading(true);
    const params = debouncedSearch
      ? `/api/v1/leads?q=${encodeURIComponent(debouncedSearch)}&limit=50`
      : "/api/v1/leads?limit=50";

    apiFetch<Lead[]>(params)
      .then(setLeads)
      .catch(() => setLeads([]))
      .finally(() => setLoading(false));
  }, [apiFetch, debouncedSearch]);

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-slate-900">Leads</h1>
      <p className="mt-1 text-sm text-slate-500">
        All discovered leads across campaigns.
      </p>

      <div className="mt-6">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name, company, or email..."
          className="w-full max-w-md rounded-lg border border-slate-300 px-4 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
        />
      </div>

      {loading ? (
        <div className="mt-6 space-y-3">
          {[1, 2, 3, 4, 5].map((i) => (
            <div key={i} className="h-12 animate-pulse rounded-lg bg-slate-100" />
          ))}
        </div>
      ) : leads.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 p-12 text-center">
          <p className="text-slate-500">
            {debouncedSearch ? "No leads match your search." : "No leads discovered yet."}
          </p>
          {!debouncedSearch && (
            <p className="mt-1 text-sm text-slate-400">
              Run the onboarding flow to discover leads for your sales plays.
            </p>
          )}
        </div>
      ) : (
        <div className="mt-6 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-slate-500">
                <th className="pb-3 pr-4 font-medium">Name</th>
                <th className="pb-3 pr-4 font-medium">Title</th>
                <th className="pb-3 pr-4 font-medium">Company</th>
                <th className="pb-3 pr-4 font-medium">Email</th>
                <th className="pb-3 font-medium">Added</th>
              </tr>
            </thead>
            <tbody>
              {leads.map((l) => (
                <tr key={l.id} className="border-b border-slate-100">
                  <td className="py-3 pr-4 font-medium text-slate-900">
                    {l.first_name} {l.last_name}
                  </td>
                  <td className="py-3 pr-4 text-slate-600">{l.title || "-"}</td>
                  <td className="py-3 pr-4 text-slate-600">{l.company || "-"}</td>
                  <td className="py-3 pr-4 text-slate-500">{l.email || "-"}</td>
                  <td className="py-3 text-slate-400">
                    {new Date(l.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
