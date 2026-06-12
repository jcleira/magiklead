"use client";

import Link from "next/link";
import { useEffect, useState, useCallback } from "react";
import { useParams } from "next/navigation";
import { useApi } from "@/hooks/use-api";

interface Campaign {
  id: string;
  name: string;
  status: string;
  play_id: string;
  sequence: unknown;
  gmail_account_id: string | null;
  stats: unknown;
  created_at: string;
}

interface CampaignLead {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  title: string;
  company: string;
  linkedin_url: string;
  status: string;
  current_step: number;
  last_sent_at: string | null;
  last_replied_at: string | null;
  created_at: string;
}

interface SequenceStep {
  step: number;
  delay_days: number;
  subject: string;
  body: string;
}

interface CampaignMetrics {
  leads_total: number;
  sent_total: number;
  sent_by_step: { step_order: number; count: number }[];
  replied_total: number;
  bounced_total: number;
  unsubscribed_total: number;
}

const statusColor: Record<string, string> = {
  active: "bg-emerald-100 text-emerald-700",
  paused: "bg-amber-100 text-amber-700",
  draft: "bg-slate-100 text-slate-600",
  queued: "bg-slate-100 text-slate-600",
  replied: "bg-blue-100 text-blue-700",
  bounced: "bg-red-100 text-red-700",
  exhausted: "bg-slate-100 text-slate-400",
};

export default function CampaignDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const { apiFetch } = useApi();

  const [campaign, setCampaign] = useState<Campaign | null>(null);
  const [leads, setLeads] = useState<CampaignLead[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [selectedLead, setSelectedLead] = useState<CampaignLead | null>(null);
  const [expandedStep, setExpandedStep] = useState<number | null>(null);

  // Email connect + deliverability popup
  const [showEmailPopup, setShowEmailPopup] = useState(false);
  const [emailAccounts, setEmailAccounts] = useState<{ id: string; email: string; provider: string }[]>([]);
  const [emailStep, setEmailStep] = useState<"connect" | "check" | "done">("connect");
  const [smtpForm, setSmtpForm] = useState({ email: "", sender_name: "", smtp_host: "", smtp_port: "587", username: "", password: "" });
  const [connectingEmail, setConnectingEmail] = useState(false);
  const [domainCheck, setDomainCheck] = useState<{
    score: number; level: string; limits: { daily_max: number; warmup_days: number };
    checks: { name: string; status: string; description: string; how_to_fix?: string }[];
  } | null>(null);

  const loadData = useCallback(() => {
    Promise.all([
      apiFetch<{ campaign: Campaign }>(`/api/v1/campaigns/${id}`),
      apiFetch<CampaignLead[]>(`/api/v1/campaigns/${id}/leads`),
    ])
      .then(([detail, campaignLeads]) => {
        setCampaign(detail.campaign);
        setLeads(campaignLeads || []);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [apiFetch, id]);

  useEffect(() => { loadData(); }, [loadData]);

  // Load email accounts
  useEffect(() => {
    apiFetch<{ id: string; email: string; provider: string }[]>("/api/v1/email-accounts")
      .then(setEmailAccounts)
      .catch(() => setEmailAccounts([]));
  }, [apiFetch]);

  async function handleConnectSMTP() {
    setConnectingEmail(true);
    try {
      await apiFetch("/api/v1/email-accounts/smtp", {
        method: "POST",
        body: JSON.stringify({ ...smtpForm, smtp_port: parseInt(smtpForm.smtp_port) || 587 }),
      });
      const accounts = await apiFetch<{ id: string; email: string; provider: string }[]>("/api/v1/email-accounts");
      setEmailAccounts(accounts);
      // Auto-check domain
      const domain = smtpForm.email.split("@")[1];
      if (domain) {
        const check = await apiFetch<typeof domainCheck>(`/api/v1/deliverability/check?domain=${domain}`);
        setDomainCheck(check);
      }
      setEmailStep("check");
    } catch (e) {
      alert("Failed: " + (e instanceof Error ? e.message : "unknown error"));
    } finally {
      setConnectingEmail(false);
    }
  }

  async function handleDiscoverLeads() {
    if (!campaign) return;
    setActionLoading("discover");
    try {
      await apiFetch("/api/v1/leads/discover", {
        method: "POST",
        body: JSON.stringify({ play_id: campaign.play_id, campaign_id: campaign.id }),
      });
      let attempts = 0;
      const poll = setInterval(async () => {
        attempts++;
        const campaignLeads = await apiFetch<CampaignLead[]>(`/api/v1/campaigns/${id}/leads`).catch(() => []);
        if (campaignLeads && campaignLeads.length > 0) {
          setLeads(campaignLeads);
          clearInterval(poll);
          setActionLoading(null);
          loadData();
        }
        if (attempts > 30) { clearInterval(poll); setActionLoading(null); }
      }, 3000);
    } catch { setActionLoading(null); }
  }

  async function handleGenerateSequence() {
    if (!campaign) return;
    setActionLoading("sequence");
    try {
      await apiFetch("/api/v1/sequences/generate", {
        method: "POST",
        body: JSON.stringify({ play_id: campaign.play_id, campaign_id: campaign.id }),
      });
      loadData();
    } catch {}
    setActionLoading(null);
  }

  async function handleStart() {
    setActionLoading("start");
    try {
      await apiFetch(`/api/v1/campaigns/${id}/start`, { method: "POST" });
      setCampaign((prev) => prev ? { ...prev, status: "active" } : prev);
    } catch {}
    setActionLoading(null);
  }

  async function handlePause() {
    setActionLoading("pause");
    try {
      await apiFetch(`/api/v1/campaigns/${id}/pause`, { method: "POST" });
      setCampaign((prev) => prev ? { ...prev, status: "paused" } : prev);
    } catch {}
    setActionLoading(null);
  }

  // Re-engage clears the reply-suppression and flips the lead back
  // to 'active' so the next worker tick resumes the sequence. Used
  // when the operator decides a detected reply was actually an
  // auto-responder (out-of-office, vacation reply).
  async function handleReengage(lead: CampaignLead) {
    const key = `reengage-${lead.id}`;
    setActionLoading(key);
    try {
      await apiFetch(`/api/v1/campaigns/${id}/leads/${lead.id}/reengage`, {
        method: "POST",
      });
      setSelectedLead(null);
      loadData();
    } catch (e) {
      alert("Re-engage failed: " + (e instanceof Error ? e.message : "unknown error"));
    } finally {
      setActionLoading(null);
    }
  }

  if (loading) {
    return (
      <div className="p-8">
        <div className="h-8 w-64 animate-pulse rounded bg-slate-100" />
        <div className="mt-8 grid gap-4 sm:grid-cols-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-20 animate-pulse rounded-xl bg-slate-100" />
          ))}
        </div>
      </div>
    );
  }

  if (!campaign) {
    return (
      <div className="p-8">
        <p className="text-slate-500">Campaign not found.</p>
        <Link href="/campaigns" className="mt-2 text-sm text-emerald-600 hover:underline">Back to campaigns</Link>
      </div>
    );
  }

  const sequence: SequenceStep[] = (() => {
    const s = campaign.sequence;
    if (Array.isArray(s)) return s;
    if (typeof s === "string") {
      try { const parsed = JSON.parse(s); return Array.isArray(parsed) ? parsed : []; }
      catch { return []; }
    }
    return [];
  })();

  const hasLeads = leads.length > 0;
  const leadsWithEmail = leads.filter((l) => !!l.email);
  const leadsWithoutEmail = leads.filter((l) => !l.email);
  const hasSequence = sequence.length > 0;
  const hasEmail = emailAccounts.length > 0;
  const isDraft = campaign.status === "draft";
  const readyToSend = leadsWithEmail.length > 0;

  // Personalize a sequence step for a specific lead
  function personalize(text: string, lead: CampaignLead): string {
    return text
      .replace(/\{\{first_name\}\}/g, lead.first_name || "there")
      .replace(/\{\{last_name\}\}/g, lead.last_name || "")
      .replace(/\{\{company\}\}/g, lead.company || "your company")
      .replace(/\{\{title\}\}/g, lead.title || "");
  }

  return (
    <div className="p-8">
      {/* Header */}
      <Link href="/campaigns" className="text-sm text-slate-400 hover:text-slate-600">&larr; Campaigns</Link>
      <div className="mt-4 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">{campaign.name}</h1>
          <span className={`mt-2 inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold capitalize ${statusColor[campaign.status] || statusColor.draft}`}>
            {campaign.status}
          </span>
        </div>
        {campaign.status === "active" && (
          <button onClick={handlePause} disabled={!!actionLoading}
            className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50">
            Pause
          </button>
        )}
      </div>

      {/* Setup Steps — shown when campaign is draft */}
      {isDraft && (
        <div className="mt-8 rounded-2xl border border-slate-200 bg-white p-6">
          <h2 className="text-lg font-semibold text-slate-900">Set up your campaign</h2>
          <p className="mt-1 text-sm text-slate-500">Complete these steps to start sending outreach.</p>
          <div className="mt-6 space-y-4">
            <SetupStep num={1} title="Discover leads" done={hasLeads}
              desc={hasLeads
                ? `${leads.length} leads discovered. ${leadsWithEmail.length} with email${leadsWithoutEmail.length > 0 ? `, ${leadsWithoutEmail.length} missing email.` : "."}`
                : "Search LinkedIn for decision-makers matching your play's ICP."}
              action={!hasLeads && (
                <button onClick={handleDiscoverLeads} disabled={!!actionLoading}
                  className="shrink-0 rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50">
                  {actionLoading === "discover" ? <Spinner text="Discovering..." /> : "Discover Leads"}
                </button>
              )} />
            <SetupStep num={2} title="Generate email sequence" done={hasSequence} disabled={!hasLeads}
              desc={hasSequence ? `${sequence.length}-step sequence ready.` : "AI writes personalized multi-step emails for your leads."}
              action={!hasSequence && hasLeads && (
                <button onClick={handleGenerateSequence} disabled={!!actionLoading}
                  className="shrink-0 rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50">
                  {actionLoading === "sequence" ? <Spinner text="Generating..." /> : "Generate Sequence"}
                </button>
              )} />
            <SetupStep num={3} title="Connect Email & Check Deliverability" done={hasEmail && !!domainCheck} disabled={!hasSequence}
              desc={hasEmail
                ? `${emailAccounts[0].email} connected.${domainCheck ? ` Score: ${domainCheck.score}/100.` : ""}`
                : "Connect your email and check domain health before sending."}
              action={!hasEmail && hasSequence ? (
                <button onClick={() => { setShowEmailPopup(true); setEmailStep("connect"); }}
                  className="shrink-0 rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800">
                  Connect Email
                </button>
              ) : hasEmail && !domainCheck && hasSequence ? (
                <button onClick={async () => {
                  const domain = emailAccounts[0].email.split("@")[1];
                  const check = await apiFetch<typeof domainCheck>(`/api/v1/deliverability/check?domain=${domain}`);
                  setDomainCheck(check);
                  setShowEmailPopup(true);
                  setEmailStep("check");
                }}
                  className="shrink-0 rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800">
                  Check Deliverability
                </button>
              ) : undefined} />
            <SetupStep num={4} title="Start campaign" done={false} disabled={!(readyToSend && hasSequence)}
              desc={readyToSend ? `Send outreach to ${leadsWithEmail.length} leads with verified emails.` : "You need leads with email addresses before you can send."}
              action={readyToSend && hasSequence && (
                <button onClick={handleStart} disabled={!!actionLoading}
                  className="shrink-0 rounded-lg bg-emerald-500 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-600 disabled:opacity-50">
                  {actionLoading === "start" ? "Starting..." : "Start Campaign"}
                </button>
              )} />
          </div>
        </div>
      )}

      {/* Per-campaign metrics — shown when not draft. Self-contained so
          the 15s poll only re-renders this block; everything below stays
          static. */}
      {!isDraft && <MetricsSection campaignId={id} />}

      {/* Leads Table */}
      {hasLeads && (
        <div className="mt-10">
          <h2 className="text-lg font-semibold text-slate-900">
            Leads <span className="text-sm font-normal text-slate-400">({leads.length})</span>
          </h2>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-slate-500">
                  <th className="pb-3 pr-4 font-medium">Name</th>
                  <th className="pb-3 pr-4 font-medium">Title</th>
                  <th className="pb-3 pr-4 font-medium">Company</th>
                  <th className="pb-3 pr-4 font-medium">Email</th>
                  <th className="pb-3 pr-4 font-medium">LinkedIn</th>
                  <th className="pb-3 pr-4 font-medium">Status</th>
                  <th className="pb-3 font-medium">Step</th>
                </tr>
              </thead>
              <tbody>
                {leads.map((l) => (
                  <tr key={l.id}
                    onClick={() => setSelectedLead(l)}
                    className="cursor-pointer border-b border-slate-100 transition-colors hover:bg-slate-50">
                    <td className="py-3.5 pr-4">
                      <div className="flex items-center gap-3">
                        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-100 text-xs font-semibold text-slate-500">
                          {(l.first_name?.[0] || "?")}{(l.last_name?.[0] || "")}
                        </div>
                        <span className="font-medium text-slate-900">{l.first_name} {l.last_name}</span>
                      </div>
                    </td>
                    <td className="py-3.5 pr-4 text-slate-600">{l.title || "-"}</td>
                    <td className="py-3.5 pr-4 text-slate-600">{l.company || "-"}</td>
                    <td className="py-3.5 pr-4">
                      {l.email ? (
                        <span className="text-slate-700">{l.email}</span>
                      ) : (
                        <span className="text-slate-300 italic">Not found</span>
                      )}
                    </td>
                    <td className="py-3.5 pr-4">
                      {l.linkedin_url && !l.linkedin_url.includes("placeholder") ? (
                        <a href={l.linkedin_url} target="_blank" rel="noopener noreferrer"
                          onClick={(e) => e.stopPropagation()}
                          className="text-blue-600 hover:underline">Profile</a>
                      ) : (
                        <span className="text-slate-300">-</span>
                      )}
                    </td>
                    <td className="py-3.5 pr-4">
                      <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold capitalize ${statusColor[l.status] || statusColor.queued}`}>
                        {l.status}
                      </span>
                    </td>
                    <td className="py-3.5 text-slate-600">
                      {l.current_step}/{sequence.length || "?"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Email Sequence */}
      {hasSequence && (
        <div className="mt-10">
          <h2 className="text-lg font-semibold text-slate-900">Email Sequence</h2>
          <div className="mt-4 space-y-3">
            {sequence.map((s, i) => {
              const isExpanded = expandedStep === i;
              return (
                <div key={i}
                  onClick={() => setExpandedStep(isExpanded ? null : i)}
                  className="cursor-pointer rounded-xl border border-slate-200 transition-all hover:border-slate-300">
                  <div className="flex items-center justify-between p-4">
                    <div className="flex items-center gap-3">
                      <div className="flex h-7 w-7 items-center justify-center rounded-full bg-slate-100 text-xs font-bold text-slate-500">
                        {s.step}
                      </div>
                      <div>
                        <p className="text-sm font-semibold text-slate-900">{s.subject}</p>
                        <p className="mt-0.5 text-xs text-slate-400">
                          {s.delay_days === 0 ? "Sent immediately" : `Sent ${s.delay_days} days after previous step`}
                        </p>
                      </div>
                    </div>
                    <svg className={`h-4 w-4 text-slate-400 transition-transform ${isExpanded ? "rotate-180" : ""}`}
                      fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                    </svg>
                  </div>
                  {isExpanded && s.body && (
                    <div className="border-t border-slate-100 bg-slate-50/50 px-4 py-4">
                      <div className="rounded-lg border border-slate-200 bg-white p-4">
                        <p className="mb-1 text-xs font-medium text-slate-400">Subject: {s.subject}</p>
                        <div className="whitespace-pre-line text-sm leading-relaxed text-slate-700">
                          {s.body}
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Email Connect + Deliverability Popup */}
      {showEmailPopup && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm"
          onClick={() => setShowEmailPopup(false)}>
          <div className="mx-4 w-full max-w-lg rounded-2xl border border-slate-200 bg-white shadow-2xl"
            onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between border-b border-slate-200 p-6">
              <h3 className="text-lg font-semibold text-slate-900">
                {emailStep === "connect" ? "Connect Email Account" : emailStep === "check" ? "Domain Health Check" : "Ready to Send"}
              </h3>
              <button onClick={() => setShowEmailPopup(false)}
                className="rounded-lg p-2 text-slate-400 hover:bg-slate-100">
                <svg className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <div className="max-h-[70vh] overflow-y-auto p-6">
              {/* Step: Connect SMTP */}
              {emailStep === "connect" && (
                <div>
                  <p className="text-sm text-slate-500">
                    Connect your email to send outreach. Works with Gmail, Outlook, or any provider with SMTP.
                  </p>
                  <div className="mt-4 space-y-3">
                    <div className="grid gap-3 sm:grid-cols-2">
                      <div>
                        <label className="text-xs font-medium text-slate-600">Email Address</label>
                        <input value={smtpForm.email} onChange={(e) => setSmtpForm({ ...smtpForm, email: e.target.value })}
                          placeholder="you@company.com"
                          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
                      </div>
                      <div>
                        <label className="text-xs font-medium text-slate-600">Sender Name</label>
                        <input value={smtpForm.sender_name} onChange={(e) => setSmtpForm({ ...smtpForm, sender_name: e.target.value })}
                          placeholder="John Doe"
                          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
                      </div>
                      <div>
                        <label className="text-xs font-medium text-slate-600">SMTP Host</label>
                        <input value={smtpForm.smtp_host} onChange={(e) => setSmtpForm({ ...smtpForm, smtp_host: e.target.value })}
                          placeholder="smtp.gmail.com"
                          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
                      </div>
                      <div>
                        <label className="text-xs font-medium text-slate-600">Port</label>
                        <input value={smtpForm.smtp_port} onChange={(e) => setSmtpForm({ ...smtpForm, smtp_port: e.target.value })}
                          placeholder="587"
                          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
                      </div>
                      <div>
                        <label className="text-xs font-medium text-slate-600">Username</label>
                        <input value={smtpForm.username} onChange={(e) => setSmtpForm({ ...smtpForm, username: e.target.value })}
                          placeholder="(usually same as email)"
                          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
                      </div>
                      <div>
                        <label className="text-xs font-medium text-slate-600">Password / App Password</label>
                        <input type="password" value={smtpForm.password} onChange={(e) => setSmtpForm({ ...smtpForm, password: e.target.value })}
                          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
                      </div>
                    </div>
                    <p className="text-xs text-slate-400">
                      For Gmail: use smtp.gmail.com, port 587, and generate an App Password at myaccount.google.com/apppasswords
                    </p>
                    <button onClick={handleConnectSMTP} disabled={connectingEmail || !smtpForm.email || !smtpForm.smtp_host || !smtpForm.password}
                      className="w-full rounded-lg bg-slate-900 py-2.5 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50">
                      {connectingEmail ? "Connecting & checking domain..." : "Connect & Check Domain"}
                    </button>
                  </div>
                </div>
              )}

              {/* Step: Domain Check Results */}
              {emailStep === "check" && domainCheck && (
                <div>
                  {/* Score */}
                  <div className={`rounded-xl p-4 ${
                    domainCheck.level === "safe" ? "bg-emerald-50 border border-emerald-200" :
                    domainCheck.level === "moderate" ? "bg-amber-50 border border-amber-200" :
                    "bg-red-50 border border-red-200"
                  }`}>
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="text-sm text-slate-500">Domain Health</p>
                        <p className={`text-3xl font-bold ${
                          domainCheck.level === "safe" ? "text-emerald-700" :
                          domainCheck.level === "moderate" ? "text-amber-700" :
                          "text-red-700"
                        }`}>{domainCheck.score}/100</p>
                      </div>
                      <div className="text-right">
                        <p className="text-sm text-slate-500">Max Daily</p>
                        <p className="text-2xl font-bold text-slate-900">{domainCheck.limits.daily_max}</p>
                        <p className="text-xs text-slate-400">{domainCheck.limits.warmup_days}d warmup</p>
                      </div>
                    </div>
                  </div>

                  {/* Checks */}
                  <div className="mt-4 space-y-2">
                    {domainCheck.checks.map((check) => (
                      <div key={check.name} className="flex items-start gap-3 rounded-lg border border-slate-100 p-3">
                        <span className={`mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                          check.status === "pass" ? "bg-emerald-100 text-emerald-600" :
                          check.status === "fail" ? "bg-red-100 text-red-600" :
                          "bg-amber-100 text-amber-600"
                        }`}>
                          {check.status === "pass" ? "✓" : check.status === "fail" ? "✕" : "!"}
                        </span>
                        <div>
                          <p className="text-sm font-medium text-slate-900">{check.name}</p>
                          <p className="text-xs text-slate-500">{check.description}</p>
                          {check.how_to_fix && (
                            <p className="mt-1 text-xs text-slate-400">{check.how_to_fix}</p>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>

                  <button onClick={() => { setEmailStep("done"); setShowEmailPopup(false); }}
                    className="mt-4 w-full rounded-lg bg-slate-900 py-2.5 text-sm font-semibold text-white hover:bg-slate-800">
                    {domainCheck.level === "safe" ? "Looks Good — Continue" :
                     domainCheck.level === "moderate" ? "Continue Anyway (recommended: fix warnings)" :
                     "Continue at Your Own Risk"}
                  </button>
                  {domainCheck.level !== "safe" && (
                    <p className="mt-2 text-center text-xs text-slate-400">
                      You can always improve your score later in the Deliverability page.
                    </p>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Lead Detail Dialog */}
      {selectedLead && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm"
          onClick={() => setSelectedLead(null)}>
          <div className="mx-4 w-full max-w-lg rounded-2xl border border-slate-200 bg-white shadow-2xl"
            onClick={(e) => e.stopPropagation()}>
            {/* Dialog Header */}
            <div className="flex items-center justify-between border-b border-slate-200 p-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-slate-100 text-lg font-semibold text-slate-500">
                  {(selectedLead.first_name?.[0] || "?")}{(selectedLead.last_name?.[0] || "")}
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-slate-900">
                    {selectedLead.first_name} {selectedLead.last_name}
                  </h3>
                  <p className="text-sm text-slate-500">
                    {selectedLead.title || "No title"} {selectedLead.company ? `at ${selectedLead.company}` : ""}
                  </p>
                </div>
              </div>
              <button onClick={() => setSelectedLead(null)}
                className="rounded-lg p-2 text-slate-400 hover:bg-slate-100 hover:text-slate-600">
                <svg className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* Dialog Body */}
            <div className="max-h-[60vh] overflow-y-auto p-6">
              {/* Contact Info */}
              <div className="space-y-3">
                <h4 className="text-xs font-semibold uppercase tracking-wider text-slate-400">Contact</h4>
                <InfoRow label="Email" value={selectedLead.email} empty="Not found" />
                <InfoRow label="LinkedIn"
                  value={selectedLead.linkedin_url && !selectedLead.linkedin_url.includes("placeholder") ? selectedLead.linkedin_url : null}
                  empty="Not available"
                  link />
                <InfoRow label="Company" value={selectedLead.company} empty="-" />
                <InfoRow label="Title" value={selectedLead.title} empty="-" />
              </div>

              {/* Campaign Status */}
              <div className="mt-6 space-y-3">
                <h4 className="text-xs font-semibold uppercase tracking-wider text-slate-400">Campaign Status</h4>
                <div className="flex items-center gap-2">
                  <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold capitalize ${statusColor[selectedLead.status] || statusColor.queued}`}>
                    {selectedLead.status}
                  </span>
                  <span className="text-sm text-slate-500">
                    &middot; Step {selectedLead.current_step} of {sequence.length}
                  </span>
                </div>
                {selectedLead.last_sent_at && (
                  <p className="text-sm text-slate-500">Last sent: {new Date(selectedLead.last_sent_at).toLocaleString()}</p>
                )}
                {selectedLead.last_replied_at && (
                  <p className="text-sm text-slate-500">Reply detected: {new Date(selectedLead.last_replied_at).toLocaleString()}</p>
                )}
                {selectedLead.status === "replied" && (
                  <div className="rounded-lg border border-amber-200 bg-amber-50 p-3">
                    <p className="text-sm font-medium text-amber-900">Sequence paused on reply</p>
                    <p className="mt-1 text-xs text-amber-700">
                      If this reply was actually an out-of-office or auto-responder, re-engage to resume the sequence.
                    </p>
                    <button
                      onClick={() => handleReengage(selectedLead)}
                      disabled={actionLoading === `reengage-${selectedLead.id}`}
                      className="mt-3 rounded-lg bg-amber-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-amber-700 disabled:opacity-50"
                    >
                      {actionLoading === `reengage-${selectedLead.id}` ? "Re-engaging…" : "Re-engage this lead"}
                    </button>
                  </div>
                )}
              </div>

              {/* No email warning */}
              {!selectedLead.email && (
                <div className="mt-6 rounded-lg border border-amber-200 bg-amber-50 p-4">
                  <p className="text-sm font-medium text-amber-800">No email address found</p>
                  <p className="mt-1 text-xs text-amber-600">
                    This lead was discovered without an email. They will be skipped when the campaign sends.
                    You can manually add their email to include them.
                  </p>
                </div>
              )}

              {/* Email Preview — only for leads with email */}
              {hasSequence && selectedLead.email && (
                <div className="mt-6 space-y-3">
                  <h4 className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                    Email Preview (personalized)
                  </h4>
                  {sequence.map((s, i) => {
                    const isSent = i < selectedLead.current_step;
                    const isCurrent = i === selectedLead.current_step;
                    return (
                      <div key={i} className={`rounded-lg border p-3 ${isCurrent ? "border-emerald-200 bg-emerald-50/50" : isSent ? "border-slate-200 bg-slate-50" : "border-slate-100"}`}>
                        <div className="flex items-center gap-2">
                          {isSent && (
                            <svg className="h-4 w-4 text-emerald-500" fill="none" stroke="currentColor" strokeWidth={3} viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                            </svg>
                          )}
                          {isCurrent && (
                            <span className="h-2 w-2 rounded-full bg-emerald-500" />
                          )}
                          <span className={`text-xs font-semibold ${isSent ? "text-slate-400" : isCurrent ? "text-emerald-700" : "text-slate-500"}`}>
                            Step {s.step} {isSent ? "(sent)" : isCurrent ? "(next)" : ""}
                          </span>
                        </div>
                        <p className="mt-1 text-sm font-medium text-slate-800">
                          {personalize(s.subject, selectedLead)}
                        </p>
                        <p className="mt-1 whitespace-pre-line text-xs leading-relaxed text-slate-500">
                          {personalize(s.body, selectedLead)}
                        </p>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/* ─── Sub-components ─── */

function SetupStep({ num, title, desc, done, disabled, action }: {
  num: number; title: string; desc: string; done: boolean; disabled?: boolean; action?: React.ReactNode;
}) {
  return (
    <div className={`flex items-start gap-4 rounded-xl border p-5 ${done ? "border-emerald-200 bg-emerald-50/50" : "border-slate-200"} ${disabled ? "opacity-50" : ""}`}>
      <div className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold ${done ? "bg-emerald-500 text-white" : "bg-slate-100 text-slate-500"}`}>
        {done ? (
          <svg className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth={3} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        ) : num}
      </div>
      <div className="flex-1">
        <p className="font-semibold text-slate-900">{title}</p>
        <p className="mt-0.5 text-sm text-slate-500">{desc}</p>
      </div>
      {action}
    </div>
  );
}

function Spinner({ text }: { text: string }) {
  return (
    <span className="flex items-center gap-2">
      <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
      </svg>
      {text}
    </span>
  );
}

function InfoRow({ label, value, empty, link }: { label: string; value: string | null; empty: string; link?: boolean }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-slate-500">{label}</span>
      {value ? (
        link ? (
          <a href={value} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
            {value.replace(/https?:\/\/(www\.)?/, "").slice(0, 40)}
          </a>
        ) : (
          <span className="font-medium text-slate-900">{value}</span>
        )
      ) : (
        <span className="italic text-slate-300">{empty}</span>
      )}
    </div>
  );
}

// MetricsSection owns its own polling lifecycle (15s tick while the
// page is open). Extracted from the parent so a metrics refresh only
// re-renders this block — the leads table, sequence list, and lead
// dialog are insulated from the tick. The interval is paused while the
// document is hidden so a backgrounded tab doesn't keep hitting the
// API; restarts on visibility-restore.
function MetricsSection({ campaignId }: { campaignId: string }) {
  const { apiFetch } = useApi();
  const [metrics, setMetrics] = useState<CampaignMetrics | null>(null);

  useEffect(() => {
    let cancelled = false;
    const fetchMetrics = () => {
      apiFetch<CampaignMetrics>(`/api/v1/campaigns/${campaignId}/metrics`)
        .then((m) => { if (!cancelled) setMetrics(m); })
        .catch(() => {});
    };
    fetchMetrics();
    let interval: number | null = window.setInterval(fetchMetrics, 15000);
    const onVisibility = () => {
      if (document.hidden) {
        if (interval !== null) { window.clearInterval(interval); interval = null; }
      } else {
        if (interval === null) {
          fetchMetrics();
          interval = window.setInterval(fetchMetrics, 15000);
        }
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      cancelled = true;
      if (interval !== null) window.clearInterval(interval);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [apiFetch, campaignId]);

  if (!metrics) {
    return (
      <div className="mt-8 grid gap-4 sm:grid-cols-3 lg:grid-cols-5">
        {[0, 1, 2, 3, 4].map((i) => (
          <div key={i} className="h-20 animate-pulse rounded-xl bg-slate-100" />
        ))}
      </div>
    );
  }

  const cards: { label: string; value: number; tone: string }[] = [
    { label: "Leads",        value: metrics.leads_total,        tone: "text-slate-900" },
    { label: "Sent",         value: metrics.sent_total,         tone: "text-slate-900" },
    { label: "Replied",      value: metrics.replied_total,      tone: "text-blue-700" },
    { label: "Bounced",      value: metrics.bounced_total,      tone: "text-red-700" },
    { label: "Unsubscribed", value: metrics.unsubscribed_total, tone: "text-amber-700" },
  ];

  return (
    <div className="mt-8">
      <div className="grid gap-4 sm:grid-cols-3 lg:grid-cols-5">
        {cards.map((c) => (
          <div key={c.label} className="rounded-xl border border-slate-200 p-4">
            <p className="text-xs text-slate-500">{c.label}</p>
            <p className={`mt-1 text-2xl font-bold ${c.tone}`}>{c.value}</p>
          </div>
        ))}
      </div>

      {metrics.sent_by_step.length > 0 && (
        <div className="mt-6 rounded-xl border border-slate-200 p-5">
          <h3 className="text-sm font-semibold text-slate-700">Sent by step</h3>
          <table className="mt-3 w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-slate-500">
                <th className="pb-2 pr-4 font-medium">Step</th>
                <th className="pb-2 font-medium">Sent</th>
              </tr>
            </thead>
            <tbody>
              {metrics.sent_by_step.map((row) => (
                <tr key={row.step_order} className="border-b border-slate-100 last:border-b-0">
                  <td className="py-2 pr-4 text-slate-700">Step {row.step_order}</td>
                  <td className="py-2 font-semibold text-slate-900">{row.count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
