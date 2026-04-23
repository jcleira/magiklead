"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

// Admin shell — paired with /api/v1/admin/* on the backend (plan §T14).
// The real authorization lives in middleware.AdminAuth on the backend
// (env var ADMIN_EMAILS). This layout doesn't gate access on its own —
// non-admin callers will simply see 403s on every fetch and the empty
// state will dominate.

const navItems = [
  { href: "/conflicts", label: "Conflicts" },
];

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  return (
    <div className="flex h-screen">
      <aside className="hidden w-60 flex-col border-r border-slate-200 bg-white lg:flex">
        <div className="flex h-16 items-center border-b border-slate-200 px-6">
          <Link href="/conflicts" className="text-lg font-bold tracking-tight">
            MagikLead <span className="text-rose-600">Admin</span>
          </Link>
        </div>
        <nav className="flex-1 space-y-1 p-3">
          {navItems.map((item) => {
            const active = pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                  active
                    ? "bg-slate-100 text-slate-900"
                    : "text-slate-500 hover:bg-slate-50 hover:text-slate-700"
                }`}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
        <div className="border-t border-slate-200 p-4">
          <Link
            href="/dashboard"
            className="text-xs text-slate-400 hover:text-slate-600"
          >
            ← Back to app
          </Link>
        </div>
      </aside>

      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-16 items-center justify-between border-b border-slate-200 bg-white px-6">
          <div className="flex items-center gap-3 lg:hidden">
            <Link href="/conflicts" className="text-lg font-bold">
              MagikLead Admin
            </Link>
          </div>
          <div className="hidden lg:block" />
          <div className="rounded-full bg-rose-50 px-3 py-1 text-xs font-medium text-rose-700">
            admin
          </div>
        </header>
        <main className="flex-1 overflow-y-auto bg-slate-50">{children}</main>
      </div>
    </div>
  );
}
