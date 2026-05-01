"use client";

import { useState } from "react";
import { useApi } from "@/hooks/use-api";

interface BusinessProfile {
  company_name: string;
  product_description: string;
  features: string[];
  pricing: string;
  target_customers: string[];
  industry: string;
}

interface Play {
  id: string;
  name: string;
  titles: string[];
  industry: string;
  company_size: string;
  signal: string;
  channels: string[];
}

export default function OnboardingPage() {
  const { apiFetch } = useApi();
  const [step, setStep] = useState(1);
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [profile, setProfile] = useState<BusinessProfile | null>(null);
  const [plays, setPlays] = useState<Play[]>([]);
  const [selectedPlays, setSelectedPlays] = useState<Set<string>>(new Set());

  async function analyzeWebsite() {
    setLoading(true);
    setError(null);

    // Auto-fix URL
    let cleanUrl = url.trim();
    if (!cleanUrl.startsWith("http://") && !cleanUrl.startsWith("https://")) {
      cleanUrl = "https://" + cleanUrl;
    }

    try {
      const result = await apiFetch<BusinessProfile>(
        "/api/v1/websites/analyze",
        {
          method: "POST",
          body: JSON.stringify({ url: cleanUrl }),
        }
      );
      setProfile(result);
      setStep(2);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function generatePlays() {
    setLoading(true);
    setError(null);
    try {
      const result = await apiFetch<Play[]>("/api/v1/plays/generate", {
        method: "POST",
      });
      if (!result || result.length === 0) {
        throw new Error(
          "The AI couldn't generate plays from your profile. Try refining the business profile and retry."
        );
      }
      setPlays(result);
      setSelectedPlays(new Set(result.map((p) => p.id)));
      setStep(3);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  function togglePlay(id: string) {
    setSelectedPlays((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function confirm() {
    setLoading(true);
    setError(null);
    try {
      // Create a campaign for each selected play.
      let firstCampaignId = "";
      for (const playId of selectedPlays) {
        const play = plays.find((p) => p.id === playId);
        const result = await apiFetch<{ id: string }>("/api/v1/campaigns", {
          method: "POST",
          body: JSON.stringify({
            play_id: playId,
            name: play?.name || "My Campaign",
          }),
        });
        if (!firstCampaignId && result?.id) {
          firstCampaignId = result.id;
        }
      }
      window.location.href = firstCampaignId
        ? `/campaigns/${firstCampaignId}`
        : "/campaigns";
    } catch (e) {
      setError((e as Error).message);
      setLoading(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl px-6 py-12">
      {/* Progress */}
      <div className="mb-10 flex items-center gap-2">
        {[1, 2, 3, 4].map((s) => (
          <div key={s} className="flex items-center gap-2">
            <div
              className={`flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold ${
                s <= step
                  ? "bg-slate-900 text-white"
                  : "bg-slate-100 text-slate-400"
              }`}
            >
              {s}
            </div>
            {s < 4 && (
              <div className={`h-0.5 w-8 ${s < step ? "bg-slate-900" : "bg-slate-200"}`} />
            )}
          </div>
        ))}
      </div>

      {error && (
        <div className="mb-6 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}

      {/* Step 1: URL */}
      {step === 1 && (
        <div>
          <h1 className="text-2xl font-bold text-slate-900">
            Welcome to MagikLead
          </h1>
          <p className="mt-2 text-slate-500">
            Paste your website URL and we&apos;ll analyze your product to generate
            sales plays.
          </p>
          <div className="mt-8">
            <label className="text-sm font-medium text-slate-700">
              Website URL
            </label>
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://yourcompany.com"
              className="mt-2 w-full rounded-lg border border-slate-300 px-4 py-3 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
            />
          </div>
          <button
            onClick={analyzeWebsite}
            disabled={url.trim().length < 4 || loading}
            className="mt-6 w-full rounded-lg bg-slate-900 py-3 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
          >
            {loading ? "Analyzing..." : "Analyze"}
          </button>
        </div>
      )}

      {/* Step 2: Profile */}
      {step === 2 && profile && (
        <div>
          <h1 className="text-2xl font-bold text-slate-900">
            Review your business profile
          </h1>
          <p className="mt-2 text-slate-500">
            Our AI generated this from your website. Edit anything that looks off.
          </p>
          <div className="mt-8 space-y-5">
            <Field label="Company Name" value={profile.company_name} />
            <Field label="Description" value={profile.product_description} multiline />
            <Field label="Features" value={profile.features.join(", ")} />
            <Field label="Pricing" value={profile.pricing} />
            <Field label="Target Customers" value={profile.target_customers.join(", ")} />
            <Field label="Industry" value={profile.industry} />
          </div>
          <div className="mt-8 flex gap-3">
            <button onClick={() => setStep(1)} className="rounded-lg border border-slate-300 px-6 py-3 text-sm font-semibold text-slate-700 hover:bg-slate-50">
              Back
            </button>
            <button
              onClick={generatePlays}
              disabled={loading}
              className="flex-1 rounded-lg bg-slate-900 py-3 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
            >
              {loading ? "Generating plays..." : "Looks good — Generate plays"}
            </button>
          </div>
        </div>
      )}

      {/* Step 3: Plays */}
      {step === 3 && (
        <div>
          <h1 className="text-2xl font-bold text-slate-900">
            Select sales plays
          </h1>
          <p className="mt-2 text-slate-500">
            We generated these outreach strategies. Select the ones you want to run.
          </p>
          <div className="mt-8 space-y-4">
            {plays.map((play) => (
              <button
                key={play.id}
                onClick={() => togglePlay(play.id)}
                className={`w-full rounded-xl border p-5 text-left transition-colors ${
                  selectedPlays.has(play.id)
                    ? "border-slate-900 bg-slate-50 ring-1 ring-slate-900"
                    : "border-slate-200 hover:border-slate-300"
                }`}
              >
                <div className="flex items-start justify-between">
                  <h3 className="font-semibold text-slate-900">{play.name}</h3>
                  <div
                    className={`flex h-5 w-5 items-center justify-center rounded border ${
                      selectedPlays.has(play.id)
                        ? "border-slate-900 bg-slate-900"
                        : "border-slate-300"
                    }`}
                  >
                    {selectedPlays.has(play.id) && (
                      <svg className="h-3 w-3 text-white" fill="none" stroke="currentColor" strokeWidth={3} viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                      </svg>
                    )}
                  </div>
                </div>
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {(play.titles || []).map((t) => (
                    <span key={t} className="rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-medium text-slate-600">
                      {t}
                    </span>
                  ))}
                </div>
                <p className="mt-2 text-sm text-slate-500">
                  {play.industry} &middot; {play.company_size}
                </p>
                <p className="mt-1 text-sm text-slate-500">
                  Signal: {play.signal}
                </p>
              </button>
            ))}
          </div>
          <div className="mt-8 flex gap-3">
            <button onClick={() => setStep(2)} className="rounded-lg border border-slate-300 px-6 py-3 text-sm font-semibold text-slate-700 hover:bg-slate-50">
              Back
            </button>
            <button
              onClick={() => setStep(4)}
              disabled={selectedPlays.size === 0}
              className="flex-1 rounded-lg bg-slate-900 py-3 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
            >
              Continue with {selectedPlays.size} play{selectedPlays.size !== 1 ? "s" : ""}
            </button>
          </div>
        </div>
      )}

      {/* Step 4: Confirm */}
      {step === 4 && (
        <div>
          <h1 className="text-2xl font-bold text-slate-900">
            Ready to discover leads
          </h1>
          <p className="mt-2 text-slate-500">
            We&apos;ll find people matching your selected plays and generate personalized email sequences.
          </p>
          <div className="mt-8 rounded-xl border border-slate-200 p-5">
            <h3 className="font-semibold text-slate-900">Summary</h3>
            <ul className="mt-3 space-y-2 text-sm text-slate-600">
              <li>{selectedPlays.size} sales play{selectedPlays.size !== 1 ? "s" : ""} selected</li>
              <li>Leads will be discovered via LinkedIn API</li>
              <li>AI will generate personalized email sequences</li>
              <li>Connect your Gmail to start sending</li>
            </ul>
          </div>
          <div className="mt-8 flex gap-3">
            <button onClick={() => setStep(3)} className="rounded-lg border border-slate-300 px-6 py-3 text-sm font-semibold text-slate-700 hover:bg-slate-50">
              Back
            </button>
            <button
              onClick={confirm}
              disabled={loading}
              className="flex-1 rounded-lg bg-slate-900 py-3 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
            >
              {loading ? "Setting up..." : "Start Discovering Leads"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function Field({
  label,
  value,
  multiline,
}: {
  label: string;
  value: string;
  multiline?: boolean;
}) {
  const Tag = multiline ? "textarea" : "input";
  return (
    <div>
      <label className="text-sm font-medium text-slate-700">{label}</label>
      <Tag
        defaultValue={value}
        rows={multiline ? 3 : undefined}
        className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
      />
    </div>
  );
}
