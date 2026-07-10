"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useApi } from "@/hooks/use-api";

interface Play {
  id: string;
  name: string;
}

interface DMStep {
  id: number; // stable local key for React; not sent to the API
  body: string;
  delay_days: number;
}

// LinkedIn caps a connection-request note at 300 characters; the
// backend rejects anything longer (see worker.NoteCharLimit).
const NOTE_LIMIT = 300;

const TOKENS = ["{{first_name}}", "{{company}}", "{{title}}"];

export default function NewLinkedInCampaignPage() {
  const { apiFetch } = useApi();
  const router = useRouter();

  const [plays, setPlays] = useState<Play[]>([]);
  const [playsLoading, setPlaysLoading] = useState(true);
  const [playId, setPlayId] = useState("");
  const [name, setName] = useState("");
  const [note, setNote] = useState("");
  const [dms, setDms] = useState<DMStep[]>([{ id: 1, body: "", delay_days: 2 }]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const nextId = useRef(2);

  useEffect(() => {
    apiFetch<Play[]>("/api/v1/plays")
      .then((p) => setPlays(p ?? []))
      .catch(() => setPlays([]))
      .finally(() => setPlaysLoading(false));
  }, [apiFetch]);

  function addDM() {
    setDms((prev) => [...prev, { id: nextId.current++, body: "", delay_days: 2 }]);
  }

  function removeDM(id: number) {
    setDms((prev) => prev.filter((d) => d.id !== id));
  }

  function moveDM(id: number, dir: -1 | 1) {
    setDms((prev) => {
      const i = prev.findIndex((d) => d.id === id);
      const j = i + dir;
      if (i < 0 || j < 0 || j >= prev.length) return prev;
      const next = [...prev];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  }

  function updateDM(id: number, patch: Partial<DMStep>) {
    setDms((prev) => prev.map((d) => (d.id === id ? { ...d, ...patch } : d)));
  }

  const noteOver = note.length > NOTE_LIMIT;
  const dmsValid = dms.length > 0 && dms.every((d) => d.body.trim().length > 0);
  const canSubmit =
    !!playId && name.trim().length > 0 && note.trim().length > 0 && !noteOver && dmsValid && !submitting;

  async function submit() {
    if (!canSubmit) return;
    setSubmitting(true);
    setError(null);
    // Step 0 is the connection note (sent with the invite, no delay);
    // steps 1+ are the DMs in their current order.
    const linkedin_sequence = [
      { step: 0, delay_days: 0, body: note.trim() },
      ...dms.map((d, i) => ({ step: i + 1, delay_days: d.delay_days, body: d.body.trim() })),
    ];
    try {
      const res = await apiFetch<{ id: string }>("/api/v1/campaigns", {
        method: "POST",
        body: JSON.stringify({
          play_id: playId,
          name: name.trim(),
          channel: "linkedin",
          linkedin_sequence,
        }),
      });
      router.push(`/campaigns/${res.id}`);
    } catch (e) {
      setError((e as Error).message);
      setSubmitting(false);
    }
  }

  const inputClass =
    "mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500";

  return (
    <div className="mx-auto max-w-2xl px-6 py-10">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900">New LinkedIn campaign</h1>
        <Link href="/campaigns" className="text-sm font-medium text-slate-500 hover:text-slate-700">
          Cancel
        </Link>
      </div>
      <p className="mt-2 text-slate-500">
        Author the connection note and the follow-up DMs. Add saved LinkedIn
        prospects from the campaign once it&apos;s created — nothing sends until you
        start it.
      </p>

      {error && (
        <div className="mt-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}

      {!playsLoading && plays.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-slate-300 p-10 text-center">
          <p className="text-slate-500">You need a sales play before creating a campaign.</p>
          <Link
            href="/onboarding"
            className="mt-4 inline-block rounded-lg bg-slate-900 px-6 py-2.5 text-sm font-semibold text-white hover:bg-slate-800"
          >
            Create a play
          </Link>
        </div>
      ) : (
        <div className="mt-8 space-y-6">
          {/* Name + play */}
          <div>
            <label className="text-sm font-medium text-slate-700">Campaign name</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Q3 founders — LinkedIn"
              className={inputClass}
            />
          </div>
          <div>
            <label className="text-sm font-medium text-slate-700">Play</label>
            <select
              value={playId}
              onChange={(e) => setPlayId(e.target.value)}
              disabled={playsLoading}
              className={inputClass}
            >
              <option value="">{playsLoading ? "Loading plays…" : "Select a play"}</option>
              {plays.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>

          {/* Token hint */}
          <div className="rounded-lg bg-slate-50 px-4 py-3 text-xs text-slate-500">
            Personalization tokens:{" "}
            {TOKENS.map((t) => (
              <code key={t} className="mr-1 rounded bg-white px-1 py-0.5 text-slate-700 ring-1 ring-slate-200">
                {t}
              </code>
            ))}
          </div>

          {/* Step 0: connection note */}
          <div className="rounded-xl border border-slate-200 p-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-slate-900">Connection note</h3>
              <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-500">
                Step 0 · sent with the invite
              </span>
            </div>
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={3}
              placeholder="Hi {{first_name}}, loved what {{company}} is building — open to connecting?"
              className={inputClass}
            />
            <div className={`mt-1 text-right text-xs ${noteOver ? "text-red-600" : "text-slate-400"}`}>
              {note.length}/{NOTE_LIMIT}
            </div>
          </div>

          {/* DM steps */}
          <div className="space-y-4">
            {dms.map((d, i) => (
              <div key={d.id} className="rounded-xl border border-slate-200 p-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-slate-900">Follow-up DM {i + 1}</h3>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => moveDM(d.id, -1)}
                      disabled={i === 0}
                      aria-label="Move step up"
                      className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30"
                    >
                      ↑
                    </button>
                    <button
                      type="button"
                      onClick={() => moveDM(d.id, 1)}
                      disabled={i === dms.length - 1}
                      aria-label="Move step down"
                      className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30"
                    >
                      ↓
                    </button>
                    <button
                      type="button"
                      onClick={() => removeDM(d.id)}
                      aria-label="Remove step"
                      className="rounded p-1 text-slate-400 hover:bg-red-50 hover:text-red-600"
                    >
                      ✕
                    </button>
                  </div>
                </div>
                <textarea
                  value={d.body}
                  onChange={(e) => updateDM(d.id, { body: e.target.value })}
                  rows={3}
                  placeholder="Thanks for connecting, {{first_name}}! Quick question about {{company}}…"
                  className={inputClass}
                />
                <div className="mt-2 flex items-center gap-2">
                  <label className="text-xs text-slate-500">Send</label>
                  <input
                    type="number"
                    min={1}
                    value={d.delay_days}
                    onChange={(e) =>
                      updateDM(d.id, { delay_days: Math.max(1, Number(e.target.value) || 1) })
                    }
                    className="w-16 rounded-lg border border-slate-300 px-2 py-1 text-sm focus:border-slate-500 focus:outline-none"
                  />
                  <span className="text-xs text-slate-500">days after the previous step</span>
                </div>
              </div>
            ))}
            <button
              type="button"
              onClick={addDM}
              className="w-full rounded-lg border border-dashed border-slate-300 py-2.5 text-sm font-medium text-slate-600 hover:border-slate-400 hover:bg-slate-50"
            >
              + Add DM step
            </button>
          </div>

          <div className="flex items-center justify-end gap-3 pt-2">
            {!dmsValid && (
              <span className="text-xs text-slate-400">Add at least one DM with a message.</span>
            )}
            <button
              type="button"
              onClick={submit}
              disabled={!canSubmit}
              className="rounded-lg bg-slate-900 px-6 py-2.5 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
            >
              {submitting ? "Creating…" : "Create campaign"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
