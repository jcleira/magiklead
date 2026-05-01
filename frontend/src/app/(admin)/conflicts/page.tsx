"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { type Conflict, useAdminApi } from "@/lib/admin-api";

const statusFilters = [
  { key: "pending", label: "Pending" },
  { key: "resolved", label: "Resolved" },
  { key: "rejected", label: "Rejected" },
  { key: "", label: "All" },
];

export default function ConflictsPage() {
  const { listConflicts, rejectConflict } = useAdminApi();
  const [status, setStatus] = useState("pending");
  const [conflicts, setConflicts] = useState<Conflict[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const reload = useCallback(() => {
    setLoading(true);
    setError(null);
    listConflicts(status)
      .then((res) => setConflicts(res.results))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [status, listConflicts]);

  useEffect(reload, [reload]);

  async function handleReject(id: string) {
    if (!confirm("Reject this conflict? It will be marked rejected without altering the canonical record.")) return;
    setBusyId(id);
    try {
      await rejectConflict(id);
      reload();
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-slate-900">Conflicts</h1>
      <p className="mt-1 text-sm text-slate-500">
        Source records that the entity resolver flagged for review (similarity ≥ 0.70 but &lt; 0.95).
      </p>

      <div className="mt-6 flex gap-2">
        {statusFilters.map((f) => (
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

      {error && (
        <div className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}

      {loading ? (
        <div className="mt-6 space-y-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-16 animate-pulse rounded-lg bg-white" />
          ))}
        </div>
      ) : conflicts.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 bg-white p-12 text-center text-slate-500">
          No conflicts in <span className="font-medium">{status || "any"}</span> state.
        </div>
      ) : (
        <ul className="mt-6 space-y-3">
          {conflicts.map((c) => {
            const expanded = expandedId === c.id;
            return (
              <li
                key={c.id}
                className="overflow-hidden rounded-lg border border-slate-200 bg-white"
              >
                <button
                  onClick={() => setExpandedId(expanded ? null : c.id)}
                  className="flex w-full items-center justify-between px-4 py-3 text-left hover:bg-slate-50"
                >
                  <div>
                    <div className="font-medium text-slate-900">
                      {c.person_canonical_name || "(no canonical person)"}
                    </div>
                    <div className="mt-0.5 text-xs text-slate-500">
                      from {c.source_name} • {new Date(c.created_at).toLocaleString()}
                    </div>
                  </div>
                  <span
                    className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                      c.status === "pending"
                        ? "bg-amber-100 text-amber-700"
                        : c.status === "resolved"
                        ? "bg-emerald-100 text-emerald-700"
                        : "bg-slate-100 text-slate-500"
                    }`}
                  >
                    {c.status}
                  </span>
                </button>
                {expanded && (
                  <div className="border-t border-slate-100 bg-slate-50 p-4">
                    <div className="grid gap-4 lg:grid-cols-2">
                      <div className="rounded border border-slate-200 bg-white p-3">
                        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
                          Canonical person
                        </div>
                        <div className="space-y-1 text-sm">
                          <div>
                            <span className="text-slate-500">Name:</span>{" "}
                            {c.person_canonical_name || "(none)"}
                          </div>
                          <div>
                            <span className="text-slate-500">ID:</span>{" "}
                            <Link
                              href={`/persons/${c.canonical_id}`}
                              className="font-mono text-xs text-rose-600 hover:underline"
                            >
                              {c.canonical_id}
                            </Link>
                          </div>
                        </div>
                      </div>
                      <div className="rounded border border-slate-200 bg-white p-3">
                        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
                          New source record ({c.source_name})
                        </div>
                        {c.source_record_fields ? (
                          <dl className="space-y-1 text-sm">
                            {Object.entries(c.source_record_fields).map(
                              ([k, v]) => (
                                <div key={k} className="flex gap-2">
                                  <dt className="text-slate-500">{k}:</dt>
                                  <dd className="font-mono text-xs text-slate-700">
                                    {String(v)}
                                  </dd>
                                </div>
                              )
                            )}
                          </dl>
                        ) : (
                          <div className="text-sm text-slate-400">No fields</div>
                        )}
                      </div>
                    </div>

                    {c.status === "pending" && (
                      <div className="mt-4 flex flex-wrap items-center gap-2">
                        <Link
                          href={`/persons/${c.canonical_id}`}
                          className="rounded-lg bg-slate-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-700"
                        >
                          Inspect person
                        </Link>
                        <button
                          onClick={() => handleReject(c.id)}
                          disabled={busyId === c.id}
                          className="rounded-lg border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-white disabled:opacity-50"
                        >
                          {busyId === c.id ? "Working…" : "Reject"}
                        </button>
                        <span className="text-xs text-slate-400">
                          To merge with another person, open the inspect view.
                        </span>
                      </div>
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
