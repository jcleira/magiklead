"use client";

import Link from "next/link";
import { useState } from "react";

const tiers = [
  {
    plan: "Free",
    monthly: 0,
    annual: 0,
    leads: "100 leads/mo",
    sequences: "300 sequences/mo",
    campaigns: "1 campaign",
    features: [
      "Email outreach only",
      "1 Gmail account",
      "AI-generated sequences",
      "Lead caching",
    ],
    cta: "Start Free",
    href: "/onboarding",
  },
  {
    plan: "Starter",
    monthly: 49,
    annual: 39,
    leads: "500 leads/mo",
    sequences: "1,500 sequences/mo",
    campaigns: "3 campaigns",
    features: [
      "Email + LinkedIn outreach",
      "1 Gmail account",
      "AI-generated sequences",
      "Lead caching",
      "Campaign analytics",
    ],
    cta: "Get Started",
    href: "/onboarding",
  },
  {
    plan: "Growth",
    monthly: 149,
    annual: 119,
    leads: "2,000 leads/mo",
    sequences: "6,000 sequences/mo",
    campaigns: "10 campaigns",
    popular: true,
    features: [
      "Email + LinkedIn outreach",
      "3 Gmail accounts",
      "AI personalization",
      "Lead caching",
      "Campaign analytics",
      "Priority lead discovery",
    ],
    cta: "Get Started",
    href: "/onboarding",
  },
  {
    plan: "Scale",
    monthly: 399,
    annual: 319,
    leads: "10,000 leads/mo",
    sequences: "30,000 sequences/mo",
    campaigns: "Unlimited",
    features: [
      "Everything in Growth",
      "Unlimited Gmail accounts",
      "API access",
      "Priority support",
      "Custom integrations",
    ],
    cta: "Get Started",
    href: "/onboarding",
  },
];

const savings = [
  {
    from: "MoneyPrinter Growth",
    fromPrice: 250,
    toPrice: 149,
    save: 101,
  },
  {
    from: "Lemlist Multichannel",
    fromPrice: 109,
    toPrice: 49,
    save: 60,
  },
  {
    from: "Apollo Pro",
    fromPrice: 99,
    toPrice: 49,
    save: 50,
  },
];

function Check() {
  return (
    <svg
      className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600"
      fill="none"
      stroke="currentColor"
      strokeWidth={3}
      viewBox="0 0 24 24"
    >
      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
    </svg>
  );
}

export default function PricingPage() {
  const [annual, setAnnual] = useState(false);

  return (
    <>
      {/* Header */}
      <section className="mx-auto max-w-4xl px-6 pb-6 pt-16 text-center">
        <h1 className="text-4xl font-bold tracking-tight text-slate-900">
          Simple, transparent pricing
        </h1>
        <p className="mt-4 text-lg text-slate-500">
          Start free. Upgrade when you need more leads.
        </p>

        {/* Toggle */}
        <div className="mt-8 flex items-center justify-center gap-3">
          <span
            className={`text-sm font-medium ${!annual ? "text-slate-900" : "text-slate-400"}`}
          >
            Monthly
          </span>
          <button
            onClick={() => setAnnual(!annual)}
            className={`relative h-6 w-11 rounded-full transition-colors ${
              annual ? "bg-slate-900" : "bg-slate-300"
            }`}
          >
            <span
              className={`absolute top-0.5 h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${
                annual ? "translate-x-5.5" : "translate-x-0.5"
              }`}
            />
          </button>
          <span
            className={`text-sm font-medium ${annual ? "text-slate-900" : "text-slate-400"}`}
          >
            Annual{" "}
            <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-semibold text-emerald-700">
              Save 20%
            </span>
          </span>
        </div>
      </section>

      {/* Tier Cards */}
      <section className="mx-auto max-w-6xl px-6 pb-20">
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {tiers.map((t) => {
            const price = annual ? t.annual : t.monthly;
            return (
              <div
                key={t.plan}
                className={`flex flex-col rounded-xl border p-6 ${
                  t.popular
                    ? "border-slate-900 ring-1 ring-slate-900"
                    : "border-slate-200"
                }`}
              >
                {t.popular && (
                  <span className="mb-3 inline-block self-start rounded-full bg-slate-900 px-3 py-0.5 text-xs font-semibold text-white">
                    Most Popular
                  </span>
                )}
                <p className="text-sm font-medium text-slate-500">{t.plan}</p>
                <p className="mt-1 text-4xl font-bold text-slate-900">
                  ${price}
                  <span className="text-base font-normal text-slate-400">
                    /mo
                  </span>
                </p>
                {annual && t.monthly > 0 && (
                  <p className="mt-1 text-xs text-slate-400 line-through">
                    ${t.monthly}/mo
                  </p>
                )}
                <div className="mt-4 space-y-1 text-sm text-slate-600">
                  <p className="font-semibold">{t.leads}</p>
                  <p>{t.sequences}</p>
                  <p>{t.campaigns}</p>
                </div>
                <ul className="mt-6 flex-1 space-y-2">
                  {t.features.map((f) => (
                    <li key={f} className="flex items-start gap-2 text-sm text-slate-600">
                      <Check />
                      {f}
                    </li>
                  ))}
                </ul>
                <Link
                  href={t.href}
                  className={`mt-6 block rounded-lg py-2.5 text-center text-sm font-semibold ${
                    t.popular
                      ? "bg-slate-900 text-white hover:bg-slate-800"
                      : "border border-slate-300 text-slate-700 hover:bg-slate-50"
                  }`}
                >
                  {t.cta}
                </Link>
              </div>
            );
          })}
        </div>
      </section>

      {/* Savings Comparison */}
      <section className="border-t border-slate-200 bg-slate-50 py-16">
        <div className="mx-auto max-w-4xl px-6">
          <h2 className="text-center text-2xl font-bold text-slate-900">
            Switch and save
          </h2>
          <div className="mt-10 grid gap-6 sm:grid-cols-3">
            {savings.map((s) => (
              <div
                key={s.from}
                className="rounded-xl border border-slate-200 bg-white p-5 text-center"
              >
                <p className="text-sm text-slate-500">Switching from</p>
                <p className="mt-1 font-semibold text-slate-700">{s.from}</p>
                <p className="text-sm text-slate-400">${s.fromPrice}/mo</p>
                <div className="my-3 text-2xl">&#8595;</div>
                <p className="font-semibold text-slate-900">
                  MagikLead
                </p>
                <p className="text-sm text-slate-600">${s.toPrice}/mo</p>
                <p className="mt-3 rounded-full bg-emerald-100 px-3 py-1 text-sm font-bold text-emerald-700">
                  Save ${s.save}/mo
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* FAQ */}
      <section className="py-16">
        <div className="mx-auto max-w-3xl px-6">
          <h2 className="text-center text-2xl font-bold text-slate-900">
            Pricing FAQ
          </h2>
          <div className="mt-10 space-y-8">
            {[
              {
                q: "What counts as a lead?",
                a: "A lead is one person discovered through our LinkedIn search. Re-discovering the same person in a different campaign doesn't count again — leads are cached.",
              },
              {
                q: "Do unused leads roll over?",
                a: "No. Lead quotas reset each billing period. This keeps pricing simple and predictable.",
              },
              {
                q: "Can I change plans anytime?",
                a: "Yes. Upgrade or downgrade from the billing portal. Changes take effect at the start of your next billing period.",
              },
              {
                q: "What payment methods do you accept?",
                a: "All major credit cards via Stripe. Annual plans are billed upfront.",
              },
            ].map((f) => (
              <div key={f.q}>
                <h3 className="font-semibold text-slate-900">{f.q}</h3>
                <p className="mt-2 text-sm text-slate-500">{f.a}</p>
              </div>
            ))}
          </div>
        </div>
      </section>
    </>
  );
}
