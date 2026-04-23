import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "MagikLead vs Apollo: Complete Comparison 2026",
  description:
    "Detailed comparison of MagikLead and Apollo for outbound sales. Compare pricing, features, email accounts, AI sequences, and credit systems side by side.",
};

const rows = [
  { feature: "Starting price", magiklead: "$49/mo", apollo: "$99/user/mo" },
  { feature: "Email outreach", magiklead: "Yes", apollo: "Yes" },
  { feature: "LinkedIn outreach", magiklead: "Yes", apollo: "Yes" },
  { feature: "Built-in leads", magiklead: "Yes", apollo: "Yes" },
  { feature: "AI sequences", magiklead: "Yes", apollo: "Limited" },
  {
    feature: "Email accounts",
    magiklead: "Your Gmail",
    apollo: "1 on Basic plan",
  },
  {
    feature: "Credit system",
    magiklead: "None — flat monthly quota",
    apollo: "Confusing credit tiers",
  },
];

export default function VsApolloPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <h1 className="text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">
        MagikLead vs Apollo: Complete Comparison
      </h1>
      <p className="mt-2 text-sm text-slate-400">Updated April 2026</p>

      {/* TL;DR */}
      <div className="mt-8 rounded-xl border border-slate-200 bg-slate-50 p-6">
        <p className="text-sm font-semibold text-slate-700">TL;DR</p>
        <p className="mt-2 text-sm text-slate-600">
          Apollo is a well-known sales platform, but its per-seat pricing and
          confusing credit system add up fast. MagikLead gives you AI-powered
          email and LinkedIn outreach with built-in lead discovery starting at
          $49/mo — no credit math, no per-seat fees.
        </p>
      </div>

      {/* Comparison Table */}
      <div className="mt-12 overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-300">
              <th className="pb-3 pr-4 font-semibold text-slate-700">
                Feature
              </th>
              <th className="pb-3 pr-4 font-semibold text-slate-900">
                MagikLead
              </th>
              <th className="pb-3 pr-4 font-semibold text-slate-500">
                Apollo
              </th>
            </tr>
          </thead>
          <tbody className="text-slate-600">
            {rows.map((row) => (
              <tr key={row.feature} className="border-b border-slate-200">
                <td className="py-3 pr-4 font-medium">{row.feature}</td>
                <td className="py-3 pr-4 font-semibold text-slate-900">
                  {row.magiklead}
                </td>
                <td className="py-3 pr-4">{row.apollo}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Who should choose */}
      <div className="mt-14 grid gap-8 sm:grid-cols-2">
        <div>
          <h2 className="text-lg font-semibold text-slate-900">
            Who should choose MagikLead
          </h2>
          <ul className="mt-4 space-y-2 text-sm text-slate-600">
            <li>
              Founders and small teams who need email + LinkedIn in one tool
            </li>
            <li>Anyone tired of calculating Apollo credit costs</li>
            <li>
              Teams that want AI-generated sequences without paying per seat
            </li>
            <li>Solo operators who want simple, flat pricing</li>
          </ul>
        </div>
        <div>
          <h2 className="text-lg font-semibold text-slate-900">
            Who should choose Apollo
          </h2>
          <ul className="mt-4 space-y-2 text-sm text-slate-600">
            <li>Large sales teams that already depend on Apollo workflows</li>
            <li>Companies that need Apollo&apos;s CRM and dialer features</li>
            <li>
              Organizations with budget for per-seat enterprise contracts
            </li>
          </ul>
        </div>
      </div>

      {/* CTA */}
      <div className="mt-16 text-center">
        <Link
          href="/onboarding"
          className="inline-block rounded-lg bg-slate-900 px-8 py-3 text-sm font-semibold text-white shadow-sm hover:bg-slate-800"
        >
          Try MagikLead Free
        </Link>
        <p className="mt-3 text-sm text-slate-400">
          No credit card required. 100 leads/mo on the free plan.
        </p>
      </div>
    </div>
  );
}
