"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { type PersonDetail, useAdminApi } from "@/lib/admin-api";

export default function PersonDetailPage() {
  const { getPerson, deletePerson } = useAdminApi();
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;

  const [data, setData] = useState<PersonDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // reload mutates state only inside async callbacks so the effect
  // body stays synchronous (lint: react-hooks/set-state-in-effect).
  // Mutation handlers that want a loading flicker call setLoading(true)
  // themselves before invoking reload.
  const reload = useCallback(async () => {
    try {
      const d = await getPerson(id);
      setData(d);
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [id, getPerson]);

  useEffect(() => {
    void reload();
  }, [reload]);

  async function handleDelete() {
    if (!confirm(
      "Hard-delete this person? CASCADE removes all evidence, employments, emails, identifiers, and aliases. This cannot be undone."
    )) return;
    setBusy(true);
    try {
      await deletePerson(id);
      router.push("/conflicts");
    } catch (e) {
      alert((e as Error).message);
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="p-8 space-y-3">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="h-20 animate-pulse rounded-lg bg-white" />
        ))}
      </div>
    );
  }
  if (error || !data) {
    return (
      <div className="p-8">
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error || "Not found"}
        </div>
        <Link href="/conflicts" className="mt-4 inline-block text-sm text-rose-600 hover:underline">
          ← Back to conflicts
        </Link>
      </div>
    );
  }

  const p = data.person;

  return (
    <div className="p-8">
      <Link
        href="/conflicts"
        className="text-xs text-slate-500 hover:text-slate-700"
      >
        ← Back to conflicts
      </Link>
      <div className="mt-2 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">{p.canonical_name}</h1>
          <p className="mt-1 font-mono text-xs text-slate-400">{p.id}</p>
        </div>
        <button
          onClick={handleDelete}
          disabled={busy}
          className="rounded-lg border border-red-300 bg-white px-3 py-2 text-sm font-medium text-red-700 hover:bg-red-50 disabled:opacity-50"
        >
          {busy ? "Deleting…" : "Hard delete"}
        </button>
      </div>

      <div className="mt-6 grid gap-4 lg:grid-cols-2">
        <Card title="Person">
          <KV label="First name" value={p.first_name} />
          <KV label="Last name" value={p.last_name} />
          <KV label="Normalized" value={p.normalized_name} />
          <KV
            label="Created"
            value={new Date(p.created_at).toLocaleString()}
          />
        </Card>

        <MergeCard survivingId={p.id} onMerged={reload} />

        <Card title={`Identifiers (${data.identifiers.length})`}>
          {data.identifiers.length === 0 && <Empty />}
          <ul className="space-y-1 text-sm">
            {data.identifiers.map((i) => (
              <li key={i.id} className="flex items-center gap-2">
                <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
                  {i.identifier_type}
                </span>
                <span className="font-mono text-xs">{i.identifier_value}</span>
                {i.is_primary && (
                  <span className="text-xs text-emerald-600">primary</span>
                )}
              </li>
            ))}
          </ul>
        </Card>

        <Card title={`Aliases (${data.aliases.length})`}>
          {data.aliases.length === 0 && <Empty />}
          <ul className="space-y-1 text-sm">
            {data.aliases.map((a) => (
              <li key={a.id} className="flex items-center gap-2">
                <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
                  {a.alias_type}
                </span>
                <span>{a.alias}</span>
              </li>
            ))}
          </ul>
        </Card>

        <Card title={`Employments (${data.employments.length})`}>
          {data.employments.length === 0 && <Empty />}
          <ul className="space-y-2 text-sm">
            {data.employments.map((e) => (
              <li key={e.id}>
                <div className="font-medium">
                  {e.title || "(no title)"}{" "}
                  <span className="text-slate-500">@ {e.organization_name}</span>
                </div>
                <div className="text-xs text-slate-400">
                  {e.is_current ? "current" : "past"} •{" "}
                  {e.start_date || "?"} – {e.end_date || "present"}
                </div>
              </li>
            ))}
          </ul>
        </Card>

        <Card title={`Emails (${data.emails.length})`}>
          {data.emails.length === 0 && <Empty />}
          <ul className="space-y-1 text-sm">
            {data.emails.map((e) => (
              <li key={e.id} className="flex items-center gap-2">
                <span className="font-mono text-xs">{e.email}</span>
                {e.verified_at ? (
                  <span className="text-xs text-emerald-600">verified</span>
                ) : (
                  <span className="text-xs text-slate-400">unverified</span>
                )}
                {e.is_catchall && (
                  <span className="text-xs text-amber-600">catchall</span>
                )}
                {e.bounce_count > 0 && (
                  <span className="text-xs text-red-600">
                    {e.bounce_count} bounce{e.bounce_count > 1 ? "s" : ""}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </Card>

        <Card title={`Phones (${data.phones.length})`}>
          {data.phones.length === 0 && <Empty />}
          <ul className="space-y-1 text-sm">
            {data.phones.map((ph) => (
              <li key={ph.id}>{ph.phone}</li>
            ))}
          </ul>
        </Card>

        <Card title={`Social profiles (${data.social_profiles.length})`}>
          {data.social_profiles.length === 0 && <Empty />}
          <ul className="space-y-1 text-sm">
            {data.social_profiles.map((s) => (
              <li key={s.id}>
                <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
                  {s.platform}
                </span>{" "}
                {s.url ? (
                  <a
                    href={s.url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-rose-600 hover:underline"
                  >
                    {s.handle || s.url}
                  </a>
                ) : (
                  <span className="text-xs">{s.handle}</span>
                )}
              </li>
            ))}
          </ul>
        </Card>
      </div>

      <div className="mt-6">
        <Card title={`Evidence (${data.evidence.length})`}>
          {data.evidence.length === 0 && <Empty />}
          {data.evidence.map((ev) => (
            <details
              key={ev.id}
              className="border-b border-slate-100 py-2 last:border-0"
            >
              <summary className="flex cursor-pointer items-center justify-between text-sm">
                <span>
                  <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
                    {ev.source_name}
                  </span>{" "}
                  <span className="text-slate-500">
                    confidence={ev.confidence}
                  </span>{" "}
                  {ev.conflict_flag && (
                    <span className="text-xs text-amber-600">conflict</span>
                  )}
                </span>
                <span className="text-xs text-slate-400">
                  {ev.ingested_at
                    ? new Date(ev.ingested_at).toLocaleDateString()
                    : ""}
                </span>
              </summary>
              <pre className="mt-2 overflow-x-auto rounded bg-slate-50 p-2 text-xs text-slate-600">
                {JSON.stringify(ev.fields, null, 2)}
              </pre>
            </details>
          ))}
        </Card>
      </div>
    </div>
  );
}

function Card({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4">
      <h2 className="mb-3 text-sm font-semibold text-slate-700">{title}</h2>
      {children}
    </div>
  );
}

function KV({ label, value }: { label: string; value: string | null }) {
  return (
    <div className="text-sm">
      <span className="text-slate-500">{label}:</span>{" "}
      <span className={value ? "text-slate-900" : "text-slate-400"}>
        {value || "—"}
      </span>
    </div>
  );
}

function Empty() {
  return <div className="text-xs text-slate-400">None.</div>;
}

// MergeCard lets the admin paste another person's UUID and either:
// 1. submit a merge with the conflict_id of an open conflict (so the
//    conflict gets marked resolved), or
// 2. submit a merge with no conflict_id — backend currently requires
//    one because the route lives under /admin/conflicts/:id/merge,
//    so we still need a conflict_id input. The admin can pull a
//    conflict id from the conflicts page.
function MergeCard({
  survivingId,
  onMerged,
}: {
  survivingId: string;
  onMerged: () => void;
}) {
  const { mergeConflict } = useAdminApi();
  const [conflictId, setConflictId] = useState("");
  const [mergedId, setMergedId] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    if (!conflictId || !mergedId) {
      setErr("conflict_id and merged_id are required");
      return;
    }
    setBusy(true);
    try {
      await mergeConflict(conflictId, {
        surviving_id: survivingId,
        merged_id: mergedId,
        reason,
      });
      setConflictId("");
      setMergedId("");
      setReason("");
      onMerged();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card title="Merge another person into this one">
      <form onSubmit={submit} className="space-y-2 text-sm">
        <input
          value={conflictId}
          onChange={(e) => setConflictId(e.target.value)}
          placeholder="conflict_id (UUID)"
          className="w-full rounded border border-slate-300 px-2 py-1 font-mono text-xs"
        />
        <input
          value={mergedId}
          onChange={(e) => setMergedId(e.target.value)}
          placeholder="merged_id — the duplicate to absorb (UUID)"
          className="w-full rounded border border-slate-300 px-2 py-1 font-mono text-xs"
        />
        <input
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="reason (optional)"
          className="w-full rounded border border-slate-300 px-2 py-1 text-xs"
        />
        {err && <div className="text-xs text-red-600">{err}</div>}
        <button
          type="submit"
          disabled={busy}
          className="rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-slate-700 disabled:opacity-50"
        >
          {busy ? "Merging…" : "Merge into this person"}
        </button>
        <p className="text-xs text-slate-400">
          Surviving: <span className="font-mono">{survivingId}</span>
        </p>
      </form>
    </Card>
  );
}
