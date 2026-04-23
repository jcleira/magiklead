import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "MagikLead vs Lemlist: Complete Comparison 2026",
  description:
    "MagikLead vs Lemlist compared. Lemlist charges $79-109 per seat. MagikLead offers email + LinkedIn outreach with built-in leads from $49/mo per account, not per user.",
};

const rows = [
  {
    feature: "Starting price",
    magiklead: "$49/mo (per account)",
    lemlist: "$79-109/user/mo (per seat)",
  },
  { feature: "Email outreach", magiklead: "Yes", lemlist: "Yes" },
  { feature: "LinkedIn outreach", magiklead: "Yes", lemlist: "Yes" },
  { feature: "AI voice messages", magiklead: "No", lemlist: "Yes" },
  { feature: "Built-in lead discovery", magiklead: "Yes", lemlist: "Limited" },
  { feature: "AI sequences", magiklead: "Yes", lemlist: "Yes" },
  {
    feature: "Pricing model",
    magiklead: "Per account, flat",
    lemlist: "Per seat, scales with team size",
  },
  {
    feature: "Email accounts",
    magiklead: "Your Gmail",
    lemlist: "5 on Expert plan",
  },
];

export default function VsLemlistPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <h1 className="text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">
        MagikLead vs Lemlist: Complete Comparison
      </h1>
      <p className="mt-2 text-sm text-slate-400">Updated April 2026</p>

      {/* TL;DR */}
      <div className="mt-8 rounded-xl border border-slate-200 bg-slate-50 p-6">
        <p className="text-sm font-semibold text-slate-700">TL;DR</p>
        <p className="mt-2 text-sm text-slate-600">
          Lemlist is a feature-rich multichannel platform with email, LinkedIn,
          and AI voice messages — but at $79-109 per seat, costs balloon as your
          team grows. MagikLead charges $49/mo per account (not per user), so
          adding teammates doesn&apos;t multiply your bill.
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
                Lemlist
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
                <td className="py-3 pr-4">{row.lemlist}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Cost example */}
      <div className="mt-10 rounded-xl border border-slate-200 p-6">
        <p className="text-sm font-semibold text-slate-700">
          Example: 3-person sales team
        </p>
        <div className="mt-3 grid grid-cols-2 gap-4 text-sm">
          <div>
            <p className="text-slate-500">MagikLead Growth</p>
            <p className="text-lg font-bold text-slate-900">$149/mo</p>
            <p className="text-xs text-slate-400">
              Same price regardless of users
            </p>
          </div>
          <div>
            <p className="text-slate-500">Lemlist Multichannel Expert</p>
            <p className="text-lg font-bold text-slate-900">$327/mo</p>
            <p className="text-xs text-slate-400">$109 x 3 seats</p>
          </div>
        </div>
      </div>

      {/* Who should choose */}
      <div className="mt-14 grid gap-8 sm:grid-cols-2">
        <div>
          <h2 className="text-lg font-semibold text-slate-900">
            Who should choose MagikLead
          </h2>
          <ul className="mt-4 space-y-2 text-sm text-slate-600">
            <li>Growing teams that want predictable costs</li>
            <li>Founders who need email + LinkedIn without per-seat pricing</li>
            <li>Anyone who wants built-in lead discovery at no extra cost</li>
            <li>Teams focused on AI-generated outreach, not manual workflows</li>
          </ul>
        </div>
        <div>
          <h2 className="text-lg font-semibold text-slate-900">
            Who should choose Lemlist
          </h2>
          <ul className="mt-4 space-y-2 text-sm text-slate-600">
            <li>Teams that need AI voice messages for outreach</li>
            <li>Solo users who value Lemlist&apos;s template marketplace</li>
            <li>
              Organizations that already have Lemlist integrated into their stack
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
