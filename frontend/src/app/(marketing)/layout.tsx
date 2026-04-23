import Link from "next/link";

function Nav() {
  return (
    <header className="absolute top-0 left-0 right-0 z-50">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
        <Link href="/" className="text-xl font-bold tracking-tight text-white">
          MagikLead
        </Link>
        <nav className="hidden items-center gap-8 text-sm font-medium text-slate-400 md:flex">
          <a href="#how-it-works" className="transition-colors hover:text-white">How It Works</a>
          <a href="#compare" className="transition-colors hover:text-white">Compare</a>
          <Link href="/pricing" className="transition-colors hover:text-white">Pricing</Link>
        </nav>
        <div className="flex items-center gap-3">
          <Link
            href="/dashboard"
            className="hidden text-sm font-medium text-slate-400 transition-colors hover:text-white sm:block"
          >
            Sign In
          </Link>
          <Link
            href="/onboarding"
            className="rounded-full bg-white/10 px-5 py-2 text-sm font-medium text-white backdrop-blur-sm transition-colors hover:bg-white/20"
          >
            Start Free
          </Link>
        </div>
      </div>
    </header>
  );
}

function Footer() {
  return (
    <footer className="border-t border-slate-800 bg-[#0a0a0a] text-slate-400">
      <div className="mx-auto max-w-6xl px-6 py-12">
        <div className="grid gap-8 sm:grid-cols-3">
          <div>
            <p className="text-lg font-bold text-white">MagikLead</p>
            <p className="mt-2 text-sm">
              AI outreach that doesn&apos;t cost a fortune.
            </p>
          </div>
          <div>
            <p className="text-sm font-semibold text-slate-300">Product</p>
            <div className="mt-3 flex flex-col gap-2 text-sm">
              <Link href="/pricing" className="hover:text-white">Pricing</Link>
              <Link href="/vs/apollo" className="hover:text-white">vs Apollo</Link>
              <Link href="/vs/instantly" className="hover:text-white">vs Instantly</Link>
              <Link href="/vs/lemlist" className="hover:text-white">vs Lemlist</Link>
            </div>
          </div>
          <div>
            <p className="text-sm font-semibold text-slate-300">Legal</p>
            <div className="mt-3 flex flex-col gap-2 text-sm">
              <Link href="/privacy" className="hover:text-white">Privacy Policy</Link>
              <Link href="/terms" className="hover:text-white">Terms of Service</Link>
              <Link href="/refund" className="hover:text-white">Refund Policy</Link>
            </div>
          </div>
        </div>
        <div className="mt-10 border-t border-slate-800 pt-6 text-center text-xs text-slate-600">
          &copy; {new Date().getFullYear()} MagikLead. All rights reserved.
        </div>
      </div>
    </footer>
  );
}

export default function MarketingLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      <Nav />
      <main className="flex-1">{children}</main>
      <Footer />
    </>
  );
}
