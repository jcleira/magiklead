"use client";

import { useState } from "react";
import { useApi } from "@/hooks/use-api";

interface CheckItem {
  name: string;
  status: "pass" | "fail" | "warn";
  description: string;
  how_to_fix?: string;
}

interface SendLimits {
  daily_max: number;
  min_delay_minutes: number;
  max_delay_minutes: number;
  warmup_days: number;
  warmup_schedule: number[];
  description: string;
}

interface DomainCheck {
  domain: string;
  score: number;
  level: "safe" | "moderate" | "risky";
  limits: SendLimits;
  checks: CheckItem[];
}

const levelConfig = {
  safe: { color: "text-emerald-700", bg: "bg-emerald-50", border: "border-emerald-200", badge: "bg-emerald-500", label: "Safe" },
  moderate: { color: "text-amber-700", bg: "bg-amber-50", border: "border-amber-200", badge: "bg-amber-500", label: "Moderate Risk" },
  risky: { color: "text-red-700", bg: "bg-red-50", border: "border-red-200", badge: "bg-red-500", label: "High Risk" },
};

const statusIcon = {
  pass: { icon: "✓", color: "text-emerald-600 bg-emerald-50" },
  fail: { icon: "✕", color: "text-red-600 bg-red-50" },
  warn: { icon: "!", color: "text-amber-600 bg-amber-50" },
};

export default function DeliverabilityPage() {
  const { apiFetch } = useApi();
  const [domain, setDomain] = useState("");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<DomainCheck | null>(null);

  async function handleCheck() {
    let d = domain.trim();
    if (d.includes("@")) d = d.split("@")[1];
    if (!d) return;

    setLoading(true);
    try {
      const data = await apiFetch<DomainCheck>(`/api/v1/deliverability/check?domain=${encodeURIComponent(d)}`);
      setResult(data);
    } catch {
      alert("Failed to check domain");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-slate-900">Email Deliverability</h1>
      <p className="mt-2 text-sm text-slate-500">
        Check your domain&apos;s email health before sending campaigns. A well-configured domain means more emails land in inboxes, not spam.
      </p>

      {/* Domain Input */}
      <div className="mt-6 flex gap-3">
        <input
          type="text"
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleCheck()}
          placeholder="yourdomain.com or you@yourdomain.com"
          className="w-full max-w-md rounded-lg border border-slate-300 px-4 py-2.5 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
        />
        <button
          onClick={handleCheck}
          disabled={loading || !domain.trim()}
          className="shrink-0 rounded-lg bg-slate-900 px-6 py-2.5 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
        >
          {loading ? "Checking..." : "Check Domain"}
        </button>
      </div>

      {result && (
        <div className="mt-8 space-y-6">
          {/* Score Card */}
          <div className={`rounded-2xl border p-6 ${levelConfig[result.level].border} ${levelConfig[result.level].bg}`}>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-slate-500">Domain Health Score</p>
                <div className="mt-1 flex items-center gap-3">
                  <span className={`text-4xl font-bold ${levelConfig[result.level].color}`}>
                    {result.score}/100
                  </span>
                  <span className={`rounded-full px-3 py-1 text-xs font-semibold text-white ${levelConfig[result.level].badge}`}>
                    {levelConfig[result.level].label}
                  </span>
                </div>
                <p className="mt-2 text-sm text-slate-600">{result.domain}</p>
              </div>
              <div className="text-right">
                <p className="text-sm font-medium text-slate-500">Sending Power</p>
                <p className={`mt-1 text-2xl font-bold ${levelConfig[result.level].color}`}>
                  {result.limits.daily_max}/day
                </p>
                <p className="mt-1 text-xs text-slate-500">after {result.limits.warmup_days}-day warmup</p>
              </div>
            </div>
          </div>

          {/* Sending Limits */}
          <div className="rounded-2xl border border-slate-200 p-6">
            <h2 className="text-lg font-semibold text-slate-900">Your Sending Limits</h2>
            <p className="mt-1 text-sm text-slate-500">{result.limits.description}</p>

            <div className="mt-4 grid gap-4 sm:grid-cols-3">
              <div className="rounded-xl bg-slate-50 p-4">
                <p className="text-xs font-medium text-slate-500">Max Daily Emails</p>
                <p className="mt-1 text-xl font-bold text-slate-900">{result.limits.daily_max}</p>
              </div>
              <div className="rounded-xl bg-slate-50 p-4">
                <p className="text-xs font-medium text-slate-500">Delay Between Emails</p>
                <p className="mt-1 text-xl font-bold text-slate-900">{result.limits.min_delay_minutes}-{result.limits.max_delay_minutes} min</p>
              </div>
              <div className="rounded-xl bg-slate-50 p-4">
                <p className="text-xs font-medium text-slate-500">Warmup Period</p>
                <p className="mt-1 text-xl font-bold text-slate-900">{result.limits.warmup_days} days</p>
              </div>
            </div>

            {/* Warmup Schedule */}
            <div className="mt-4">
              <p className="text-xs font-medium text-slate-500">Warmup Schedule (emails per day)</p>
              <div className="mt-2 flex gap-1">
                {result.limits.warmup_schedule.map((n, i) => (
                  <div key={i} className="group relative">
                    <div
                      className="w-3 rounded-sm bg-slate-300 transition-colors hover:bg-slate-500"
                      style={{ height: `${Math.max(8, (n / result.limits.daily_max) * 60)}px` }}
                    />
                    <div className="absolute bottom-full left-1/2 mb-1 -translate-x-1/2 rounded bg-slate-900 px-1.5 py-0.5 text-[10px] text-white opacity-0 group-hover:opacity-100">
                      Day {i + 1}: {n}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>

          {/* DNS Checks */}
          <div className="rounded-2xl border border-slate-200 p-6">
            <h2 className="text-lg font-semibold text-slate-900">DNS Configuration</h2>
            <p className="mt-1 text-sm text-slate-500">
              These records tell email servers that you&apos;re authorized to send email from this domain.
              Fix any failing checks to improve deliverability and unlock higher sending limits.
            </p>

            <div className="mt-4 space-y-3">
              {result.checks.map((check) => (
                <div key={check.name} className="rounded-xl border border-slate-200 p-4">
                  <div className="flex items-start gap-3">
                    <div className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-sm font-bold ${statusIcon[check.status].color}`}>
                      {statusIcon[check.status].icon}
                    </div>
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <p className="font-semibold text-slate-900">{check.name}</p>
                        <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase ${
                          check.status === "pass" ? "bg-emerald-100 text-emerald-700" :
                          check.status === "fail" ? "bg-red-100 text-red-700" :
                          "bg-amber-100 text-amber-700"
                        }`}>
                          {check.status}
                        </span>
                      </div>
                      <p className="mt-1 text-sm text-slate-600">{check.description}</p>
                      {check.how_to_fix && (
                        <div className="mt-2 rounded-lg bg-slate-50 p-3">
                          <p className="text-xs font-medium text-slate-500">How to fix:</p>
                          <p className="mt-1 text-xs text-slate-700">{check.how_to_fix}</p>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Guide */}
          <div className="rounded-2xl border border-slate-200 p-6">
            <h2 className="text-lg font-semibold text-slate-900">Email Deliverability Guide</h2>
            <p className="mt-1 text-sm text-slate-500">Follow these best practices to maximize inbox placement.</p>

            <div className="mt-4 space-y-4">
              <GuideSection
                num="1"
                title="Set up DNS records (SPF, DKIM, DMARC)"
                description="These prove to email servers that you're authorized to send. Without them, your emails go to spam."
                items={[
                  "SPF — tells servers which IPs can send for your domain",
                  "DKIM — cryptographically signs your emails to prove they weren't tampered with",
                  "DMARC — tells servers what to do with emails that fail SPF/DKIM checks",
                ]}
              />
              <GuideSection
                num="2"
                title="Use a dedicated sending email"
                description="Don't use your personal email for outreach. Create a dedicated one like outreach@yourdomain.com."
                items={[
                  "Protects your personal email reputation",
                  "If one gets flagged, your main email is safe",
                  "Looks more professional to recipients",
                ]}
              />
              <GuideSection
                num="3"
                title="Warmup your email account"
                description="New email accounts have no reputation. We automatically ramp up your sending volume over 2-4 weeks."
                items={[
                  "Week 1: 5-15 emails/day",
                  "Week 2: 15-30 emails/day",
                  "Week 3: 30-50 emails/day",
                  "Week 4+: Full volume (based on your domain health score)",
                ]}
              />
              <GuideSection
                num="4"
                title="Write good emails"
                description="Even with perfect technical setup, bad content gets flagged."
                items={[
                  "Personalize every email (we do this with AI)",
                  "Keep it short — under 150 words",
                  "Don't use spam trigger words (FREE, GUARANTEED, ACT NOW)",
                  "Include a clear unsubscribe option (we add this automatically)",
                  "Don't include too many links (1-2 max)",
                ]}
              />
            </div>
          </div>
        </div>
      )}

      {/* Show guide even without checking a domain */}
      {!result && (
        <div className="mt-10 rounded-2xl border border-slate-200 p-6">
          <h2 className="text-lg font-semibold text-slate-900">Before You Send</h2>
          <p className="mt-1 text-sm text-slate-500">
            Cold email deliverability is the difference between landing in the inbox and landing in spam.
            Check your domain above to see your current health score and what to fix.
          </p>
          <div className="mt-6 grid gap-4 sm:grid-cols-3">
            <div className="rounded-xl border border-emerald-200 bg-emerald-50 p-5 text-center">
              <p className="text-2xl font-bold text-emerald-700">70-100</p>
              <p className="mt-1 text-sm font-semibold text-emerald-700">Safe</p>
              <p className="mt-2 text-xs text-emerald-600">Up to 100 emails/day. Full power after 14-day warmup.</p>
            </div>
            <div className="rounded-xl border border-amber-200 bg-amber-50 p-5 text-center">
              <p className="text-2xl font-bold text-amber-700">40-69</p>
              <p className="mt-1 text-sm font-semibold text-amber-700">Moderate</p>
              <p className="mt-2 text-xs text-amber-600">Up to 50 emails/day. Fix DNS issues to unlock more.</p>
            </div>
            <div className="rounded-xl border border-red-200 bg-red-50 p-5 text-center">
              <p className="text-2xl font-bold text-red-700">0-39</p>
              <p className="mt-1 text-sm font-semibold text-red-700">Risky</p>
              <p className="mt-2 text-xs text-red-600">Limited to 20 emails/day. Fix DNS before sending.</p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function GuideSection({ num, title, description, items }: { num: string; title: string; description: string; items: string[] }) {
  return (
    <div className="rounded-xl border border-slate-100 p-4">
      <div className="flex items-start gap-3">
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-900 text-xs font-bold text-white">
          {num}
        </div>
        <div>
          <p className="font-semibold text-slate-900">{title}</p>
          <p className="mt-1 text-sm text-slate-500">{description}</p>
          <ul className="mt-2 space-y-1">
            {items.map((item) => (
              <li key={item} className="flex items-start gap-2 text-sm text-slate-600">
                <span className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-slate-400" />
                {item}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  );
}
