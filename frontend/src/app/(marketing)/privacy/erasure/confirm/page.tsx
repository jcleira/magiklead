"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

type Status = "pending" | "success" | "error";

function ConfirmInner() {
  const params = useSearchParams();
  const token = params.get("token");
  const [status, setStatus] = useState<Status>("pending");
  const [result, setResult] = useState<{
    deleted_persons: number;
    blocked_types: string[];
  } | null>(null);
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (!token) {
      setStatus("error");
      setMessage("Missing confirmation token.");
      return;
    }
    (async () => {
      try {
        const res = await fetch(
          `${API_URL}/api/v1/privacy/erasure/confirm?token=${encodeURIComponent(token)}`,
          { method: "POST" }
        );
        const body = await res.json().catch(() => null);
        if (!res.ok) {
          throw new Error(body?.message || `Request failed (${res.status})`);
        }
        setResult(body);
        setStatus("success");
      } catch (e) {
        setStatus("error");
        setMessage((e as Error).message);
      }
    })();
  }, [token]);

  if (status === "pending") {
    return <p className="mt-4 text-slate-600">Confirming your request…</p>;
  }
  if (status === "success" && result) {
    return (
      <div className="mt-4 rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-900">
        <p className="font-medium">Done.</p>
        <p className="mt-2">
          {result.deleted_persons === 0
            ? "No matching canonical record was found — nothing to delete."
            : `Deleted ${result.deleted_persons} canonical record(s).`}
          {result.blocked_types.length > 0 && (
            <>
              {" "}
              Added to the ingest blocklist: {result.blocked_types.join(", ")}.
            </>
          )}
        </p>
      </div>
    );
  }
  return (
    <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      {message}
    </div>
  );
}

export default function ErasureConfirmPage() {
  return (
    <main className="mx-auto max-w-xl px-6 py-16">
      <h1 className="text-3xl font-semibold text-slate-900">
        Erasure confirmation
      </h1>
      <Suspense fallback={<p className="mt-4 text-slate-600">Loading…</p>}>
        <ConfirmInner />
      </Suspense>
    </main>
  );
}
