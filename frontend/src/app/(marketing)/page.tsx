import Link from "next/link";

/* ─── Data ─── */

const competitors = [
  { name: "MagikLead", price: "$49", priceSub: "/mo", email: true, leads: true, ai: "Full", accounts: "Your Gmail", highlight: true },
  { name: "MoneyPrinter", price: "$250", priceSub: "/mo", email: true, leads: true, ai: "Full", accounts: "$3/mo each", highlight: false },
  { name: "Apollo", price: "$99", priceSub: "/user", email: true, leads: true, ai: "Limited", accounts: "1 (Basic)", highlight: false },
  { name: "Instantly", price: "$47", priceSub: "/mo", email: true, leads: false, ai: "Full", accounts: "Unlimited", highlight: false },
  { name: "Lemlist", price: "$109", priceSub: "/user", email: true, leads: false, ai: "Full", accounts: "5 (Expert)", highlight: false },
];

const featureRows = [
  { label: "Email outreach", key: "email" as const },
  { label: "Built-in lead discovery", key: "leads" as const },
];

const faqs = [
  { q: "How does lead discovery work?", a: "We search a 1B+ person B2B database (People Data Labs) for people matching your ideal customer profile, find their verified email addresses, and cache every lookup in our canonical graph — repeat searches across tenants cost nothing." },
  { q: "Do I need a separate email tool?", a: "No. You connect your Gmail account via OAuth, and we send emails directly from your inbox. Better deliverability, no extra cost." },
  { q: "Is my Gmail safe?", a: "Yes. We only request send and read permissions. We never store your email content. Daily send limits are enforced to protect your account." },
  { q: "How does billing work?", a: "Start free with 100 leads/month. Upgrade anytime. Plans are based on lead volume, not seats. Cancel anytime." },
];

const tiers = [
  { plan: "Free", price: "$0", leads: "100 leads/mo", cta: "Start Free", href: "/onboarding" },
  { plan: "Starter", price: "$49", leads: "500 leads/mo", cta: "Get Started", href: "/onboarding" },
  { plan: "Growth", price: "$149", leads: "2,000 leads/mo", cta: "Get Started", href: "/onboarding", popular: true },
  { plan: "Scale", price: "$399", leads: "10,000 leads/mo", cta: "Get Started", href: "/pricing" },
];

/* ─── Icons ─── */

function CheckIcon({ className = "" }: { className?: string }) {
  return (
    <svg className={`h-5 w-5 ${className}`} viewBox="0 0 20 20" fill="currentColor">
      <path fillRule="evenodd" d="M16.704 4.153a.75.75 0 01.143 1.052l-8 10.5a.75.75 0 01-1.127.075l-4.5-4.5a.75.75 0 011.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 011.05-.143z" clipRule="evenodd" />
    </svg>
  );
}

function XIcon() {
  return (
    <svg className="h-5 w-5 text-slate-300" viewBox="0 0 20 20" fill="currentColor">
      <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
    </svg>
  );
}

function ArrowRight() {
  return (
    <svg className="ml-2 h-4 w-4 transition-transform group-hover:translate-x-1" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3" />
    </svg>
  );
}

/* ─── Page ─── */

export default function LandingPage() {
  return (
    <>
      {/* ── Hero ── */}
      <section className="relative overflow-hidden bg-[#0a0a0a] text-white">
        {/* Subtle grid pattern */}
        <div
          className="pointer-events-none absolute inset-0 opacity-[0.03]"
          style={{
            backgroundImage: "linear-gradient(#fff 1px, transparent 1px), linear-gradient(90deg, #fff 1px, transparent 1px)",
            backgroundSize: "64px 64px",
          }}
        />
        {/* Radial glow */}
        <div className="pointer-events-none absolute left-1/2 top-0 h-[600px] w-[800px] -translate-x-1/2 rounded-full bg-emerald-500/10 blur-3xl" />

        <div className="relative mx-auto max-w-5xl px-6 pb-24 pt-24 sm:pb-32 sm:pt-32">
          <div className="animate-fade-up text-center">
            <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-4 py-1.5 text-sm text-emerald-400">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
              50% cheaper than the competition
            </div>

            <h1 className="text-5xl font-bold tracking-tight sm:text-6xl lg:text-7xl">
              AI outreach that
              <br />
              <span className="font-[var(--font-display)] italic text-emerald-400">doesn&apos;t cost a fortune</span>
            </h1>

            <p className="animate-fade-up delay-100 mx-auto mt-6 max-w-xl text-lg leading-relaxed text-slate-400">
              Paste your website. Get verified leads. Send personalized emails
              from your own Gmail. Starting at $49/mo.
            </p>

            <div className="animate-fade-up delay-200 mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
              <Link
                href="/onboarding"
                className="group inline-flex items-center rounded-full bg-emerald-500 px-8 py-3.5 text-sm font-semibold text-white shadow-lg shadow-emerald-500/25 transition-all hover:bg-emerald-400 hover:shadow-emerald-400/30"
              >
                Start Free
                <ArrowRight />
              </Link>
              <a
                href="#how-it-works"
                className="rounded-full border border-white/15 px-8 py-3.5 text-sm font-medium text-slate-300 transition-colors hover:border-white/30 hover:text-white"
              >
                See How It Works
              </a>
            </div>
          </div>

          {/* Trust signal */}
          <p className="animate-fade-in delay-500 mt-16 text-center text-xs tracking-wide text-slate-600">
            NO CREDIT CARD REQUIRED &nbsp;&middot;&nbsp; 100 FREE LEADS &nbsp;&middot;&nbsp; CANCEL ANYTIME
          </p>
        </div>
      </section>

      {/* ── How It Works ── */}
      <section id="how-it-works" className="relative bg-white py-24">
        <div className="mx-auto max-w-5xl px-6">
          <p className="text-center text-sm font-semibold uppercase tracking-widest text-emerald-600">
            How It Works
          </p>
          <h2 className="mt-3 text-center text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">
            From zero to outreach in 5 minutes
          </h2>

          <div className="mt-16 grid gap-0 sm:grid-cols-3">
            {[
              {
                num: "01",
                title: "Paste your website",
                desc: "Our AI scrapes and analyzes your product, pricing, and target customers. No manual setup.",
                icon: (
                  <svg className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418" />
                  </svg>
                ),
              },
              {
                num: "02",
                title: "Discover leads",
                desc: "We pull decision-makers matching your ICP from a 1B+ person B2B database, verify their emails, and cache every lookup.",
                icon: (
                  <svg className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
                  </svg>
                ),
              },
              {
                num: "03",
                title: "Send sequences",
                desc: "AI writes personalized multi-step emails. We send them from your own Gmail — better deliverability, $0 cost.",
                icon: (
                  <svg className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5" />
                  </svg>
                ),
              },
            ].map((step, i) => (
              <div key={step.num} className="relative flex flex-col items-center px-8 py-10 text-center">
                {/* Connector line */}
                {i < 2 && (
                  <div className="absolute right-0 top-1/2 hidden h-px w-8 -translate-y-1/2 bg-slate-200 sm:block" />
                )}
                <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-slate-900 text-white">
                  {step.icon}
                </div>
                <span className="mt-4 font-mono text-xs font-semibold tracking-wider text-emerald-600">
                  STEP {step.num}
                </span>
                <h3 className="mt-2 text-lg font-semibold text-slate-900">
                  {step.title}
                </h3>
                <p className="mt-2 text-sm leading-relaxed text-slate-500">
                  {step.desc}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Value Props ── */}
      <section className="border-y border-slate-100 bg-slate-50/50 py-24">
        <div className="mx-auto max-w-5xl px-6">
          <p className="text-center text-sm font-semibold uppercase tracking-widest text-emerald-600">
            Why MagikLead
          </p>
          <h2 className="mt-3 text-center text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">
            Built for founders who ship fast
          </h2>

          <div className="mt-14 grid gap-6 sm:grid-cols-3">
            {[
              {
                title: "All-in-one pipeline",
                desc: "Leads, email sequences, send, reply detection, suppression — in one tool. Stop duct-taping Apollo, Instantly, and a deliverability hack together.",
                icon: (
                  <svg className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z" />
                  </svg>
                ),
              },
              {
                title: "50% cheaper",
                desc: "MoneyPrinter charges $250/mo. Apollo charges per seat. We start at $49/mo flat. Same AI, half the price.",
                icon: (
                  <svg className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z" />
                  </svg>
                ),
              },
              {
                title: "No gotchas",
                desc: "No credits that run out. No per-seat pricing. No managed mailbox fees. You see exactly what you pay.",
                icon: (
                  <svg className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
                  </svg>
                ),
              },
            ].map((v) => (
              <div
                key={v.title}
                className="group rounded-2xl border border-slate-200 bg-white p-8 shadow-sm transition-shadow hover:shadow-md"
              >
                <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 transition-colors group-hover:bg-emerald-100">
                  {v.icon}
                </div>
                <h3 className="mt-5 text-lg font-semibold text-slate-900">{v.title}</h3>
                <p className="mt-2 text-sm leading-relaxed text-slate-500">{v.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Comparison Table ── */}
      <section id="compare" className="bg-white py-24">
        <div className="mx-auto max-w-5xl px-6">
          <p className="text-center text-sm font-semibold uppercase tracking-widest text-emerald-600">
            Compare
          </p>
          <h2 className="mt-3 text-center text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">
            See exactly what you get
          </h2>
          <p className="mx-auto mt-4 max-w-xl text-center text-sm text-slate-500">
            Real pricing as of April 2026. No hidden fees, no credit tricks.
          </p>

          <div className="mt-12 overflow-x-auto rounded-2xl border border-slate-200">
            <table className="w-full min-w-[700px] text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50">
                  <th className="p-4 text-left font-medium text-slate-500">Feature</th>
                  {competitors.map((c) => (
                    <th
                      key={c.name}
                      className={`p-4 text-center font-semibold ${
                        c.highlight
                          ? "bg-emerald-50 text-emerald-700"
                          : "text-slate-600"
                      }`}
                    >
                      {c.name}
                      {c.highlight && (
                        <span className="ml-1.5 inline-block h-1.5 w-1.5 rounded-full bg-emerald-500" />
                      )}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {/* Price row — bold */}
                <tr className="border-b border-slate-100">
                  <td className="p-4 font-medium text-slate-700">Price</td>
                  {competitors.map((c) => (
                    <td
                      key={c.name}
                      className={`p-4 text-center ${
                        c.highlight
                          ? "bg-emerald-50 text-lg font-bold text-emerald-700"
                          : "font-medium text-slate-600"
                      }`}
                    >
                      {c.price}
                      <span className="text-xs font-normal text-slate-400">{c.priceSub}</span>
                    </td>
                  ))}
                </tr>
                {/* Feature rows */}
                {featureRows.map((row) => (
                  <tr key={row.key} className="border-b border-slate-100">
                    <td className="p-4 font-medium text-slate-700">{row.label}</td>
                    {competitors.map((c) => (
                      <td
                        key={c.name}
                        className={`p-4 text-center ${c.highlight ? "bg-emerald-50" : ""}`}
                      >
                        {c[row.key] ? (
                          <CheckIcon className={c.highlight ? "mx-auto text-emerald-600" : "mx-auto text-slate-400"} />
                        ) : (
                          <span className="mx-auto block"><XIcon /></span>
                        )}
                      </td>
                    ))}
                  </tr>
                ))}
                {/* AI row */}
                <tr className="border-b border-slate-100">
                  <td className="p-4 font-medium text-slate-700">AI sequences</td>
                  {competitors.map((c) => (
                    <td key={c.name} className={`p-4 text-center text-sm ${c.highlight ? "bg-emerald-50 font-medium text-emerald-700" : "text-slate-500"}`}>
                      {c.ai}
                    </td>
                  ))}
                </tr>
                {/* Accounts row */}
                <tr>
                  <td className="p-4 font-medium text-slate-700">Email accounts</td>
                  {competitors.map((c) => (
                    <td key={c.name} className={`p-4 text-center text-sm ${c.highlight ? "bg-emerald-50 font-medium text-emerald-700" : "text-slate-500"}`}>
                      {c.accounts}
                    </td>
                  ))}
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ── Pricing Preview ── */}
      <section className="border-t border-slate-100 bg-slate-50/50 py-24">
        <div className="mx-auto max-w-5xl px-6 text-center">
          <p className="text-sm font-semibold uppercase tracking-widest text-emerald-600">
            Pricing
          </p>
          <h2 className="mt-3 text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">
            Simple, transparent pricing
          </h2>
          <p className="mt-4 text-slate-500">
            Start free. Upgrade when you need more leads.
          </p>

          <div className="mt-12 grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
            {tiers.map((t) => (
              <div
                key={t.plan}
                className={`relative flex flex-col rounded-2xl border bg-white p-7 text-left shadow-sm ${
                  t.popular
                    ? "border-emerald-500 ring-1 ring-emerald-500"
                    : "border-slate-200"
                }`}
              >
                {t.popular && (
                  <span className="absolute -top-3 left-1/2 -translate-x-1/2 rounded-full bg-emerald-500 px-4 py-1 text-xs font-semibold text-white">
                    Most Popular
                  </span>
                )}
                <p className="text-sm font-semibold uppercase tracking-wider text-slate-400">{t.plan}</p>
                <p className="mt-2 text-4xl font-bold text-slate-900">
                  {t.price}
                  <span className="text-base font-normal text-slate-400">/mo</span>
                </p>
                <p className="mt-3 text-sm text-slate-500">{t.leads}</p>
                <Link
                  href={t.href}
                  className={`mt-8 block rounded-full py-2.5 text-center text-sm font-semibold transition-colors ${
                    t.popular
                      ? "bg-emerald-500 text-white hover:bg-emerald-600"
                      : "border border-slate-200 text-slate-700 hover:border-slate-300 hover:bg-slate-50"
                  }`}
                >
                  {t.cta}
                </Link>
              </div>
            ))}
          </div>

          <Link
            href="/pricing"
            className="mt-8 inline-flex items-center text-sm font-medium text-emerald-600 hover:text-emerald-700"
          >
            See full feature comparison
            <svg className="ml-1 h-4 w-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3" />
            </svg>
          </Link>
        </div>
      </section>

      {/* ── FAQ ── */}
      <section className="bg-white py-24">
        <div className="mx-auto max-w-2xl px-6">
          <h2 className="text-center text-3xl font-bold tracking-tight text-slate-900">
            Frequently asked questions
          </h2>

          <div className="mt-14 divide-y divide-slate-200">
            {faqs.map((f) => (
              <div key={f.q} className="py-6">
                <h3 className="text-base font-semibold text-slate-900">{f.q}</h3>
                <p className="mt-3 text-sm leading-relaxed text-slate-500">{f.a}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Final CTA ── */}
      <section className="relative overflow-hidden bg-[#0a0a0a] py-24 text-white">
        <div className="pointer-events-none absolute left-1/2 top-1/2 h-[500px] w-[700px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-emerald-500/10 blur-3xl" />
        <div className="relative mx-auto max-w-3xl px-6 text-center">
          <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">
            Ready to find your
            <br />
            <span className="font-[var(--font-display)] italic text-emerald-400">next customers?</span>
          </h2>
          <p className="mt-4 text-slate-400">
            Start free — no credit card required. 100 leads on us.
          </p>
          <Link
            href="/onboarding"
            className="group mt-8 inline-flex items-center rounded-full bg-emerald-500 px-8 py-3.5 text-sm font-semibold text-white shadow-lg shadow-emerald-500/25 transition-all hover:bg-emerald-400 hover:shadow-emerald-400/30"
          >
            Start Free
            <ArrowRight />
          </Link>
        </div>
      </section>
    </>
  );
}
