"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  LeadsApiError,
  type LeadSearchResult,
  type LinkedInSearchRequest,
  type LinkedInSearchResponse,
  type LinkedInSearchResult,
  type SavedLead,
  useLeadsApi,
} from "@/lib/leads-api";

const PAGE_SIZE = 25;

type Tab = "search" | "saved";

// Where the Search tab looks: LinkedIn Sales Navigator (through the
// connected account, on demand) or the people MagikLead already stores.
type Source = "linkedin" | "database";

export default function LeadsPage() {
  const [tab, setTab] = useState<Tab>("search");

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-slate-900">Leads</h1>
      <p className="mt-1 text-sm text-slate-500">
        Find people, then save the ones you want to reach. Only people with a
        LinkedIn profile can join a LinkedIn campaign.
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

interface SearchFilters {
  titles: string[];
  industries: string[];
  locations: string[];
  companySize: string;
}

const inputClass =
  "rounded-lg border border-slate-300 px-4 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500";

function SearchTab() {
  const [source, setSource] = useState<Source>("linkedin");
  const [titlesInput, setTitlesInput] = useState("");
  const [industriesInput, setIndustriesInput] = useState("");
  const [locationsInput, setLocationsInput] = useState("");
  const [companySize, setCompanySize] = useState("");

  const titles = useMemo(() => splitCSV(titlesInput), [titlesInput]);
  const industries = useMemo(
    () => splitCSV(industriesInput),
    [industriesInput]
  );
  const locations = useMemo(() => splitCSV(locationsInput), [locationsInput]);
  const filters: SearchFilters = { titles, industries, locations, companySize };

  return (
    <div>
      <div className="inline-flex rounded-lg border border-slate-200 bg-slate-50 p-1 text-sm">
        <SourceButton
          active={source === "linkedin"}
          onClick={() => setSource("linkedin")}
        >
          LinkedIn Sales Navigator
        </SourceButton>
        <SourceButton
          active={source === "database"}
          onClick={() => setSource("database")}
        >
          People in MagikLead
        </SourceButton>
      </div>

      <div className="mt-4 flex flex-wrap items-end gap-4">
        <label className="flex flex-col gap-1">
          <span className="text-xs font-medium text-slate-500">
            Titles (comma-separated)
          </span>
          <input
            type="text"
            value={titlesInput}
            onChange={(e) => setTitlesInput(e.target.value)}
            placeholder="Managing Partner, Founding Partner"
            className={`min-w-80 ${inputClass}`}
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
            placeholder="Legal Services, Accounting"
            className={`min-w-64 ${inputClass}`}
          />
        </label>

        <label className="flex flex-col gap-1">
          <span className="text-xs font-medium text-slate-500">
            Company size
          </span>
          <select
            value={companySize}
            onChange={(e) => setCompanySize(e.target.value)}
            className={`bg-white ${inputClass}`}
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
            placeholder="United States, Madrid"
            className={`min-w-64 ${inputClass}`}
          />
        </label>
      </div>

      {source === "linkedin" ? (
        <LinkedInSearch filters={filters} />
      ) : (
        <DatabaseSearch filters={filters} />
      )}
    </div>
  );
}

function SourceButton({
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
      type="button"
      onClick={onClick}
      className={`rounded-md px-3 py-1.5 font-medium transition-colors ${
        active
          ? "bg-white text-slate-900 shadow-sm ring-1 ring-slate-200"
          : "text-slate-500 hover:text-slate-700"
      }`}
    >
      {children}
    </button>
  );
}

// useSaveLead tracks which results the user saved in this session.
function useSaveLead() {
  const { saveLead } = useLeadsApi();
  const [savedIds, setSavedIds] = useState<Set<string>>(new Set());
  const [savingId, setSavingId] = useState<string | null>(null);

  async function save(personId: string) {
    setSavingId(personId);
    try {
      await saveLead(personId);
      setSavedIds((prev) => new Set(prev).add(personId));
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setSavingId(null);
    }
  }

  return { savedIds, savingId, save };
}

function SaveButton({
  personId,
  saved,
  saving,
  onSave,
}: {
  personId: string;
  saved: boolean;
  saving: boolean;
  onSave: (personId: string) => void;
}) {
  return (
    <button
      onClick={() => onSave(personId)}
      disabled={saved || saving}
      className="rounded-lg border border-slate-300 px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
    >
      {saved ? "Saved" : saving ? "Saving…" : "Save lead"}
    </button>
  );
}

// LinkedInSearch runs a Sales Navigator people search as the connected
// LinkedIn account. It runs only on a click: each search uses the
// account's daily profile budget, and LinkedIn watches the volume.
function LinkedInSearch({ filters }: { filters: SearchFilters }) {
  const { searchLinkedIn, ready } = useLeadsApi();
  const { savedIds, savingId, save } = useSaveLead();
  const [query, setQuery] = useState<LinkedInSearchRequest | null>(null);
  const [results, setResults] = useState<LinkedInSearchResult[]>([]);
  const [hidden, setHidden] = useState(0);
  const [last, setLast] = useState<LinkedInSearchResponse | null>(null);
  const [loading, setLoading] = useState<"search" | "more" | null>(null);
  const [error, setError] = useState<{ message: string; code: string } | null>(
    null
  );

  const hasFilter =
    filters.titles.length > 0 ||
    filters.industries.length > 0 ||
    filters.locations.length > 0 ||
    filters.companySize !== "";

  async function run(req: LinkedInSearchRequest, more: boolean) {
    setLoading(more ? "more" : "search");
    setError(null);
    try {
      const res = await searchLinkedIn(req);
      setResults((prev) => (more ? [...prev, ...res.results] : res.results));
      setHidden((prev) => (more ? prev + res.hidden : res.hidden));
      setLast(res);
    } catch (e) {
      const code = e instanceof LeadsApiError ? e.code : "unknown";
      setError({ message: (e as Error).message, code });
    } finally {
      setLoading(null);
    }
  }

  function handleSearch() {
    const req: LinkedInSearchRequest = {
      titles: filters.titles,
      industries: filters.industries,
      locations: filters.locations,
      company_size: filters.companySize || undefined,
    };
    setQuery(req);
    setResults([]);
    setHidden(0);
    setLast(null);
    void run(req, false);
  }

  function handleMore() {
    if (!query || !last?.cursor) return;
    void run({ ...query, cursor: last.cursor }, true);
  }

  const matched = last
    ? [
        ...last.locations.map((m) => ({ kind: "Location", ...m })),
        ...last.industries.map((m) => ({ kind: "Industry", ...m })),
      ]
    : [];

  return (
    <div>
      <div className="mt-5 flex flex-wrap items-center gap-3">
        <button
          onClick={handleSearch}
          disabled={!ready || !hasFilter || loading !== null}
          className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
        >
          {loading === "search" ? "Searching…" : "Search LinkedIn"}
        </button>
        <span className="text-xs text-slate-500">
          Runs as your connected LinkedIn account, with Sales Navigator.
          {last &&
            ` ${last.used_today} of ${last.daily_cap} profiles used in the last 24 hours.`}
        </span>
      </div>
      {!hasFilter && (
        <p className="mt-2 text-xs text-slate-400">
          Add a title, an industry, a location or a company size first.
        </p>
      )}

      {error && (
        <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error.message}
          {(error.code === "linkedin_not_connected" ||
            error.code === "linkedin_restricted") && (
            <>
              {" "}
              <Link href="/settings" className="font-semibold underline">
                Open Settings
              </Link>
            </>
          )}
        </div>
      )}

      {matched.length > 0 && (
        <div className="mt-4 flex flex-wrap gap-2 text-xs text-slate-500">
          {matched.map((m) => (
            <span
              key={`${m.kind}-${m.id}`}
              className="rounded-full bg-slate-100 px-2.5 py-1"
            >
              {m.kind}: {m.query} → {m.title}
            </span>
          ))}
        </div>
      )}

      {loading === "search" ? (
        <ResultsSkeleton />
      ) : !last && results.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 p-12 text-center text-slate-500">
          Set the filters, then search LinkedIn. Every person found has a
          LinkedIn profile, so the LinkedIn rail can invite them.
        </div>
      ) : results.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 p-12 text-center text-slate-500">
          LinkedIn found nobody with a public profile for these filters.
        </div>
      ) : (
        <div className="mt-6 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-slate-500">
                <th className="pb-3 pr-4 font-medium">Name</th>
                <th className="pb-3 pr-4 font-medium">Title</th>
                <th className="pb-3 pr-4 font-medium">Company</th>
                <th className="pb-3 pr-4 font-medium">Location</th>
                <th className="pb-3 pr-4 font-medium">Profile</th>
                <th className="pb-3 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {results.map((r) => (
                <tr key={r.person_id} className="border-b border-slate-100">
                  <td className="py-3 pr-4 font-medium text-slate-900">
                    {r.name}
                    {r.pending_invitation && (
                      <span
                        className="ml-2 rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 ring-1 ring-inset ring-amber-200"
                        title="Your LinkedIn account already invited this person"
                      >
                        Invite pending
                      </span>
                    )}
                  </td>
                  <td className="py-3 pr-4 text-slate-600">{r.title || "—"}</td>
                  <td className="py-3 pr-4 text-slate-600">
                    {r.company || "—"}
                  </td>
                  <td className="py-3 pr-4 text-slate-500">
                    {r.location || "—"}
                  </td>
                  <td className="py-3 pr-4 text-sm">
                    <LinkedInLink url={r.linkedin_url} />
                  </td>
                  <td className="py-3">
                    <SaveButton
                      personId={r.person_id}
                      saved={savedIds.has(r.person_id)}
                      saving={savingId === r.person_id}
                      onSave={save}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {last && (
        <div className="mt-4 flex items-center justify-between text-sm">
          <span className="text-slate-500">
            {results.length} {results.length === 1 ? "person" : "people"}
            {last.total > 0 && ` of about ${last.total.toLocaleString()}`}
            {hidden > 0 &&
              ` · ${hidden} left out: LinkedIn hides their profile`}
          </span>
          {last.cursor && (
            <button
              onClick={handleMore}
              disabled={loading !== null}
              className="rounded-lg border border-slate-300 px-3 py-1 font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
            >
              {loading === "more" ? "Loading…" : "More results"}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

// DatabaseSearch searches the people MagikLead already stores. It runs
// as you type (it costs nothing).
function DatabaseSearch({ filters }: { filters: SearchFilters }) {
  const { searchLeads } = useLeadsApi();
  const { savedIds, savingId, save } = useSaveLead();
  const [description, setDescription] = useState("");
  const [withEmail, setWithEmail] = useState(false);
  // On when the page opens: a lead with no LinkedIn profile cannot join a
  // LinkedIn campaign.
  const [withLinkedIn, setWithLinkedIn] = useState(true);
  const [offset, setOffset] = useState(0);
  const [results, setResults] = useState<LeadSearchResult[]>([]);
  const [pdlCalled, setPdlCalled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

  // Debounce input so every keystroke doesn't fire a search.
  const { titles, industries, locations, companySize } = filters;
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
        with_linkedin: withLinkedIn,
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
    withLinkedIn,
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
    withLinkedIn,
  ]);

  return (
    <div>
      <div className="mt-4 space-y-4">
        <label className="flex flex-col gap-1">
          <span className="text-xs font-medium text-slate-500">
            ICP description (free text)
          </span>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder="Heads of Marketing at European B2B SaaS, 50–500 employees"
            className={inputClass}
          />
        </label>

        <div className="flex flex-wrap gap-6">
          <label className="flex items-center gap-2 text-sm text-slate-700">
            <input
              type="checkbox"
              checked={withLinkedIn}
              onChange={(e) => setWithLinkedIn(e.target.checked)}
              className="h-4 w-4 rounded border-slate-300"
            />
            With LinkedIn profile
          </label>
          <label className="flex items-center gap-2 text-sm text-slate-700">
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
        <ResultsSkeleton />
      ) : results.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 p-12 text-center text-slate-500">
          No matches. Try broader titles, or turn off a filter. To find new
          people, search LinkedIn Sales Navigator.
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
                {results.map((r) => (
                  <tr key={r.person_id} className="border-b border-slate-100">
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
                      <LinkedInLink url={r.linkedin_url} />
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
                      <SaveButton
                        personId={r.person_id}
                        saved={savedIds.has(r.person_id)}
                        saving={savingId === r.person_id}
                        onSave={save}
                      />
                    </td>
                  </tr>
                ))}
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

function ResultsSkeleton() {
  return (
    <div className="mt-6 space-y-3">
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={i} className="h-12 animate-pulse rounded-lg bg-slate-100" />
      ))}
    </div>
  );
}

// LinkedInLink shows the profile link, or marks a person the LinkedIn
// rail cannot invite.
function LinkedInLink({ url }: { url?: string }) {
  if (!url) {
    return (
      <span
        className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-500"
        title="No LinkedIn profile: a LinkedIn campaign cannot invite this person"
      >
        No LinkedIn profile
      </span>
    );
  }
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className="font-medium text-sky-600 hover:text-sky-800 hover:underline"
    >
      LinkedIn ↗
    </a>
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

  const withoutProfile = leads.filter((l) => !l.linkedin_url).length;

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
        {withoutProfile > 0 &&
          ` · ${withoutProfile} without a LinkedIn profile (a LinkedIn campaign cannot invite them)`}
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
                <th className="pb-3 pr-4 font-medium">Title</th>
                <th className="pb-3 pr-4 font-medium">Company</th>
                <th className="pb-3 pr-4 font-medium">Profile</th>
                <th className="pb-3 pr-4 font-medium">Status</th>
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
                  <td className="py-3 pr-4 text-slate-600">{l.title || "—"}</td>
                  <td className="py-3 pr-4 text-slate-600">
                    {l.company || "—"}
                  </td>
                  <td className="py-3 pr-4 text-sm">
                    <LinkedInLink url={l.linkedin_url} />
                  </td>
                  <td className="py-3 pr-4 text-slate-600">{l.status}</td>
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
