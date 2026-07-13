"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  type LeadSearchResult,
  type SavedLead,
  useLeadsApi,
} from "@/lib/leads-api";

const PAGE_SIZE = 25;

type Tab = "search" | "saved";

export default function LeadsPage() {
  const [tab, setTab] = useState<Tab>("search");

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-slate-900">Leads</h1>
      <p className="mt-1 text-sm text-slate-500">
        Search the canonical person graph, then save the ones you want to
        work.
      </p>

      <div className="mt-6 border-b border-slate-200">
        <nav className="flex gap-4 text-sm">
          <TabButton active={tab === "search"} onClick={() => setTab("search")}>
            Search
          </TabButton>
          <TabButton active={tab === "saved"} onClick={() => setTab("saved")}>
            Saved
          </TabButton>
        </nav>
      </div>

      <div className="mt-6">
        {tab === "search" ? <SearchTab /> : <SavedTab />}
      </div>
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`border-b-2 px-1 pb-3 font-medium transition-colors ${
        active
          ? "border-slate-900 text-slate-900"
          : "border-transparent text-slate-500 hover:text-slate-700"
      }`}
    >
      {children}
    </button>
  );
}

// ---- Search tab ------------------------------------------------------------

const COMPANY_SIZE_OPTIONS = [
  { value: "", label: "Any size" },
  { value: "1-10", label: "1–10" },
  { value: "11-50", label: "11–50" },
  { value: "51-200", label: "51–200" },
  { value: "201-500", label: "201–500" },
  { value: "501-1000", label: "501–1,000" },
  { value: "1001-5000", label: "1,001–5,000" },
  { value: "5001-10000", label: "5,001–10,000" },
  { value: "10001+", label: "10,001+" },
];

function SearchTab() {
  const { searchLeads, saveLead } = useLeadsApi();
  const [titlesInput, setTitlesInput] = useState("");
  const [industriesInput, setIndustriesInput] = useState("");
  const [locationsInput, setLocationsInput] = useState("");
  const [companySize, setCompanySize] = useState("");
  const [description, setDescription] = useState("");
  const [withEmail, setWithEmail] = useState(false);
  const [offset, setOffset] = useState(0);
  const [results, setResults] = useState<LeadSearchResult[]>([]);
  const [pdlCalled, setPdlCalled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedIds, setSavedIds] = useState<Set<string>>(new Set());
  const [savingId, setSavingId] = useState<string | null>(null);

  // On first mount, seed the description from the onboarding-captured
  // ICP description (if the user just came through onboarding); the
  // key is persisted to localStorage by the onboarding flow.
  useEffect(() => {
    if (typeof window === "undefined") return;
    const seeded = window.localStorage.getItem("magiklead.icp_description");
    if (seeded && !description) {
      setDescription(seeded);
    }
    // Intentionally one-shot — only on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const titles = useMemo(() => splitCSV(titlesInput), [titlesInput]);
  const industries = useMemo(
    () => splitCSV(industriesInput),
    [industriesInput]
  );
  const locations = useMemo(() => splitCSV(locationsInput), [locationsInput]);

  // Debounce input so every keystroke doesn't fire a search.
  const [debouncedTitles, setDebouncedTitles] = useState<string[]>([]);
  const [debouncedIndustries, setDebouncedIndustries] = useState<string[]>([]);
  const [debouncedLocations, setDebouncedLocations] = useState<string[]>([]);
  const [debouncedDescription, setDebouncedDescription] = useState("");
  useEffect(() => {
    const t = setTimeout(() => {
      setDebouncedTitles(titles);
      setDebouncedIndustries(industries);
      setDebouncedLocations(locations);
      setDebouncedDescription(description.trim());
    }, 350);
    return () => clearTimeout(t);
  }, [titles, industries, locations, description]);

  const runSearch = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await searchLeads({
        titles: debouncedTitles.length ? debouncedTitles : undefined,
        industries: debouncedIndustries.length
          ? debouncedIndustries
          : undefined,
        locations: debouncedLocations.length ? debouncedLocations : undefined,
        company_size: companySize || undefined,
        description: debouncedDescription || undefined,
        with_email: withEmail,
        limit: PAGE_SIZE,
        offset,
      });
      setResults(res.results);
      setPdlCalled(Boolean(res.pdl_called));
    } catch (e) {
      setError((e as Error).message);
      setResults([]);
      setPdlCalled(false);
    } finally {
      setLoading(false);
    }
  }, [
    searchLeads,
    debouncedTitles,
    debouncedIndustries,
    debouncedLocations,
    companySize,
    debouncedDescription,
    withEmail,
    offset,
  ]);

  useEffect(() => {
    void runSearch();
  }, [runSearch]);

  // Reset pagination when filters change.
  useEffect(() => {
    setOffset(0);
  }, [
    debouncedTitles,
    debouncedIndustries,
    debouncedLocations,
    companySize,
    debouncedDescription,
    withEmail,
  ]);

  async function handleSave(personId: string) {
    setSavingId(personId);
    try {
      await saveLead(personId);
      setSavedIds((prev) => {
        const next = new Set(prev);
        next.add(personId);
        return next;
      });
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setSavingId(null);
    }
  }

  return (
    <div>
      <div className="space-y-4">
        <label className="flex flex-col gap-1">
          <span className="text-xs font-medium text-slate-500">
            ICP description (free text)
          </span>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder="Heads of Marketing at European B2B SaaS, 50–500 employees"
            className="rounded-lg border border-slate-300 px-4 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
          />
        </label>

        <div className="flex flex-wrap items-end gap-4">
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-slate-500">
              Titles (comma-separated)
            </span>
            <input
              type="text"
              value={titlesInput}
              onChange={(e) => setTitlesInput(e.target.value)}
              placeholder="VP Sales, Head of Growth"
              className="min-w-80 rounded-lg border border-slate-300 px-4 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
            />
          </label>

          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-slate-500">
              Industries (comma-separated)
            </span>
            <input
              type="text"
              value={industriesInput}
              onChange={(e) => setIndustriesInput(e.target.value)}
              placeholder="software, fintech"
              className="min-w-64 rounded-lg border border-slate-300 px-4 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
            />
          </label>

          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-slate-500">
              Company size
            </span>
            <select
              value={companySize}
              onChange={(e) => setCompanySize(e.target.value)}
              className="rounded-lg border border-slate-300 bg-white px-4 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
            >
              {COMPANY_SIZE_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
          </label>

          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-slate-500">
              Locations (comma-separated)
            </span>
            <input
              type="text"
              value={locationsInput}
              onChange={(e) => setLocationsInput(e.target.value)}
              placeholder="Berlin, Madrid"
              className="min-w-64 rounded-lg border border-slate-300 px-4 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
            />
          </label>

          <label className="flex items-center gap-2 pb-2 text-sm text-slate-700">
            <input
              type="checkbox"
              checked={withEmail}
              onChange={(e) => setWithEmail(e.target.checked)}
              className="h-4 w-4 rounded border-slate-300"
            />
            With verified email
          </label>
        </div>
      </div>

      {pdlCalled && (
        <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
          Live data fetched from People Data Labs and cached for future searches.
        </div>
      )}

      {error && (
        <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}

      {loading ? (
        <div className="mt-6 space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div
              key={i}
              className="h-12 animate-pulse rounded-lg bg-slate-100"
            />
          ))}
        </div>
      ) : results.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 p-12 text-center text-slate-500">
          No matches. Try broader titles, or turn off the verified-email
          filter.
        </div>
      ) : (
        <>
          <div className="mt-6 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-slate-500">
                  <th className="pb-3 pr-4 font-medium">Name</th>
                  <th className="pb-3 pr-4 font-medium">Title</th>
                  <th className="pb-3 pr-4 font-medium">Organization</th>
                  <th className="pb-3 pr-4 font-medium">Profile</th>
                  <th className="pb-3 pr-4 font-medium">Email</th>
                  <th className="pb-3 font-medium"></th>
                </tr>
              </thead>
              <tbody>
                {results.map((r) => {
                  const saved = savedIds.has(r.person_id);
                  return (
                    <tr
                      key={r.person_id}
                      className="border-b border-slate-100"
                    >
                      <td className="py-3 pr-4 font-medium text-slate-900">
                        {r.name}
                      </td>
                      <td className="py-3 pr-4 text-slate-600">
                        {r.title || "—"}
                      </td>
                      <td className="py-3 pr-4 text-slate-600">
                        {r.organization_name}
                        {r.domain && (
                          <span className="ml-2 text-xs text-slate-400">
                            {r.domain}
                          </span>
                        )}
                      </td>
                      <td className="py-3 pr-4 text-sm">
                        {r.linkedin_url ? (
                          <a
                            href={r.linkedin_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="font-medium text-sky-600 hover:text-sky-800 hover:underline"
                          >
                            LinkedIn ↗
                          </a>
                        ) : (
                          <span className="text-slate-400">—</span>
                        )}
                      </td>
                      <td className="py-3 pr-4 text-sm">
                        {r.email ? (
                          <div className="flex items-center gap-2">
                            <span className="text-slate-700">{r.email}</span>
                            <EmailBadge
                              verified={r.email_verified}
                              catchall={r.email_is_catchall}
                            />
                          </div>
                        ) : (
                          <span className="text-slate-400">—</span>
                        )}
                      </td>
                      <td className="py-3">
                        <button
                          onClick={() => handleSave(r.person_id)}
                          disabled={saved || savingId === r.person_id}
                          className="rounded-lg border border-slate-300 px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
                        >
                          {saved
                            ? "Saved"
                            : savingId === r.person_id
                            ? "Saving…"
                            : "Save lead"}
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          <div className="mt-4 flex items-center justify-between text-sm">
            <span className="text-slate-500">
              Showing {offset + 1}–{offset + results.length}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                disabled={offset === 0 || loading}
                className="rounded-lg border border-slate-300 px-3 py-1 font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
              >
                Previous
              </button>
              <button
                onClick={() => setOffset(offset + PAGE_SIZE)}
                disabled={results.length < PAGE_SIZE || loading}
                className="rounded-lg border border-slate-300 px-3 py-1 font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function EmailBadge({
  verified,
  catchall,
}: {
  verified: boolean;
  catchall: boolean;
}) {
  if (verified) {
    return (
      <span
        className="rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 ring-1 ring-inset ring-emerald-200"
        title="Verified by PDL or SMTP probe"
      >
        Verified
      </span>
    );
  }
  if (catchall) {
    return (
      <span
        className="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 ring-1 ring-inset ring-amber-200"
        title="Catch-all domain; deliverability unknown"
      >
        Catch-all
      </span>
    );
  }
  return (
    <span
      className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600 ring-1 ring-inset ring-slate-200"
      title="Not yet verified"
    >
      Unverified
    </span>
  );
}

function splitCSV(s: string): string[] {
  return s
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
}

// ---- Saved tab -------------------------------------------------------------

const STATUS_FILTERS = [
  { key: "", label: "All" },
  { key: "new", label: "New" },
  { key: "contacted", label: "Contacted" },
  { key: "replied", label: "Replied" },
  { key: "archived", label: "Archived" },
];

function SavedTab() {
  const { listSavedLeads, removeSavedLead } = useLeadsApi();
  const [status, setStatus] = useState("");
  const [leads, setLeads] = useState<SavedLead[]>([]);
  const [count, setCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await listSavedLeads({
        status: status || undefined,
        limit: 100,
      });
      setLeads(res.results);
      setCount(res.count);
    } catch (e) {
      setError((e as Error).message);
      setLeads([]);
    } finally {
      setLoading(false);
    }
  }, [listSavedLeads, status]);

  useEffect(() => {
    void reload();
  }, [reload]);

  async function handleRemove(personId: string) {
    if (!confirm("Remove this lead from your saved list?")) return;
    setBusyId(personId);
    try {
      await removeSavedLead(personId);
      setLeads((prev) => prev.filter((l) => l.person_id !== personId));
      setCount((c) => Math.max(0, c - 1));
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div>
      <div className="flex gap-2">
        {STATUS_FILTERS.map((f) => (
          <button
            key={f.key}
            onClick={() => setStatus(f.key)}
            className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
              status === f.key
                ? "bg-slate-900 text-white"
                : "bg-white text-slate-600 hover:bg-slate-100"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      <div className="mt-3 text-xs text-slate-400">
        {count} saved {count === 1 ? "lead" : "leads"}
      </div>

      {error && (
        <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}

      {loading ? (
        <div className="mt-6 space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              className="h-12 animate-pulse rounded-lg bg-slate-100"
            />
          ))}
        </div>
      ) : leads.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 p-12 text-center text-slate-500">
          Nothing saved in{" "}
          <span className="font-medium">{status || "any"}</span> yet. Use the
          Search tab to save leads.
        </div>
      ) : (
        <div className="mt-6 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-slate-500">
                <th className="pb-3 pr-4 font-medium">Name</th>
                <th className="pb-3 pr-4 font-medium">Status</th>
                <th className="pb-3 pr-4 font-medium">Notes</th>
                <th className="pb-3 pr-4 font-medium">Saved</th>
                <th className="pb-3 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {leads.map((l) => (
                <tr key={l.person_id} className="border-b border-slate-100">
                  <td className="py-3 pr-4 font-medium text-slate-900">
                    {l.name || `${l.first_name ?? ""} ${l.last_name ?? ""}`}
                  </td>
                  <td className="py-3 pr-4 text-slate-600">{l.status}</td>
                  <td className="py-3 pr-4 text-slate-500">
                    {l.notes || "—"}
                  </td>
                  <td className="py-3 pr-4 text-slate-400">
                    {new Date(l.added_at).toLocaleDateString()}
                  </td>
                  <td className="py-3">
                    <button
                      onClick={() => handleRemove(l.person_id)}
                      disabled={busyId === l.person_id}
                      className="rounded-lg border border-slate-300 px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
                    >
                      {busyId === l.person_id ? "Removing…" : "Remove"}
                    </button>
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
