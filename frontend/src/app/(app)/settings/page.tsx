"use client";

import { useEffect, useState } from "react";
import { useApi } from "@/hooks/use-api";

interface EmailAccount {
  id: string;
  email: string;
  provider: string;
  smtp_host: string;
}

export default function SettingsPage() {
  const { apiFetch } = useApi();
  const [upgrading, setUpgrading] = useState(false);
  const [emailAccounts, setEmailAccounts] = useState<EmailAccount[]>([]);
  const [showSMTPForm, setShowSMTPForm] = useState(false);
  const [connectingEmail, setConnectingEmail] = useState(false);
  const [smtpForm, setSmtpForm] = useState({
    email: "", sender_name: "", smtp_host: "", smtp_port: "587", username: "", password: "",
  });

  useEffect(() => {
    apiFetch<EmailAccount[]>("/api/v1/email-accounts")
      .then(setEmailAccounts)
      .catch(() => setEmailAccounts([]));
  }, [apiFetch]);

  async function handleConnectSMTP() {
    setConnectingEmail(true);
    try {
      await apiFetch("/api/v1/email-accounts/smtp", {
        method: "POST",
        body: JSON.stringify({
          ...smtpForm,
          smtp_port: parseInt(smtpForm.smtp_port) || 587,
        }),
      });
      const accounts = await apiFetch<EmailAccount[]>("/api/v1/email-accounts");
      setEmailAccounts(accounts);
      setShowSMTPForm(false);
      setSmtpForm({ email: "", sender_name: "", smtp_host: "", smtp_port: "587", username: "", password: "" });
    } catch (e) {
      alert("Failed to connect: " + (e instanceof Error ? e.message : "unknown error"));
    } finally {
      setConnectingEmail(false);
    }
  }

  async function handleDeleteAccount(id: string) {
    await apiFetch(`/api/v1/email-accounts/${id}`, { method: "DELETE" }).catch(() => {});
    setEmailAccounts((prev) => prev.filter((a) => a.id !== id));
  }

  async function handleUpgrade(priceId: string) {
    setUpgrading(true);
    try {
      const { url } = await apiFetch<{ url: string }>("/api/v1/billing/checkout", {
        method: "POST",
        body: JSON.stringify({ price_id: priceId }),
      });
      window.location.href = url;
    } catch {
      alert("Failed to start checkout. Please try again.");
    } finally {
      setUpgrading(false);
    }
  }

  async function handleManageBilling() {
    try {
      const { url } = await apiFetch<{ url: string }>("/api/v1/billing/portal", {
        method: "POST",
      });
      window.location.href = url;
    } catch {
      alert("No active subscription to manage.");
    }
  }

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-slate-900">Settings</h1>

      {/* Billing */}
      <section className="mt-10">
        <h2 className="text-lg font-semibold text-slate-900">Billing</h2>
        <div className="mt-4 rounded-xl border border-slate-200 p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-500">Current plan</p>
              <p className="mt-1 text-xl font-bold text-slate-900">Free</p>
              <p className="mt-1 text-sm text-slate-400">
                100 leads/mo &middot; 300 sequences/mo
              </p>
            </div>
            <button
              onClick={handleManageBilling}
              className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50"
            >
              Manage Billing
            </button>
          </div>

          <div className="mt-6 border-t border-slate-200 pt-6">
            <p className="text-sm font-medium text-slate-700">Usage this period</p>
            <div className="mt-3 grid gap-4 sm:grid-cols-2">
              <div>
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">Leads</span>
                  <span className="font-medium text-slate-900">0 / 100</span>
                </div>
                <div className="mt-1.5 h-2 rounded-full bg-slate-100">
                  <div className="h-2 rounded-full bg-slate-900" style={{ width: "0%" }} />
                </div>
              </div>
              <div>
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">Sequences</span>
                  <span className="font-medium text-slate-900">0 / 300</span>
                </div>
                <div className="mt-1.5 h-2 rounded-full bg-slate-100">
                  <div className="h-2 rounded-full bg-slate-900" style={{ width: "0%" }} />
                </div>
              </div>
            </div>
          </div>

          <div className="mt-6 border-t border-slate-200 pt-6">
            <p className="text-sm font-medium text-slate-700">Upgrade</p>
            <div className="mt-3 grid gap-3 sm:grid-cols-3">
              {[
                { name: "Starter", price: "$49/mo", leads: "500 leads", id: "price_starter" },
                { name: "Growth", price: "$149/mo", leads: "2,000 leads", id: "price_growth", popular: true },
                { name: "Scale", price: "$399/mo", leads: "10,000 leads", id: "price_scale" },
              ].map((plan) => (
                <button
                  key={plan.id}
                  onClick={() => handleUpgrade(plan.id)}
                  disabled={upgrading}
                  className={`rounded-lg border p-4 text-left transition-colors hover:border-slate-400 disabled:opacity-50 ${
                    plan.popular ? "border-slate-900" : "border-slate-200"
                  }`}
                >
                  <p className="text-sm font-semibold text-slate-900">{plan.name}</p>
                  <p className="mt-0.5 text-lg font-bold text-slate-900">{plan.price}</p>
                  <p className="mt-1 text-xs text-slate-500">{plan.leads}</p>
                </button>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Email Accounts */}
      <section className="mt-10">
        <h2 className="text-lg font-semibold text-slate-900">Email Accounts</h2>
        <p className="mt-1 text-sm text-slate-500">
          Connect your email to send outreach. Works with any provider (Gmail, Outlook, custom domain).
        </p>

        {/* Connected accounts */}
        <div className="mt-4 space-y-3">
          {emailAccounts.map((acc) => (
            <div key={acc.id} className="flex items-center justify-between rounded-xl border border-slate-200 p-4">
              <div>
                <p className="text-sm font-medium text-slate-900">{acc.email}</p>
                <p className="text-xs text-slate-400">{acc.provider.toUpperCase()} &middot; {acc.smtp_host || "Gmail OAuth"}</p>
              </div>
              <button
                onClick={() => handleDeleteAccount(acc.id)}
                className="text-sm text-red-500 hover:text-red-700"
              >
                Remove
              </button>
            </div>
          ))}
        </div>

        {/* Add SMTP */}
        {!showSMTPForm ? (
          <button
            onClick={() => setShowSMTPForm(true)}
            className="mt-4 rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800"
          >
            Connect Email Account
          </button>
        ) : (
          <div className="mt-4 rounded-xl border border-slate-200 p-6">
            <h3 className="text-sm font-semibold text-slate-900">Connect via SMTP</h3>
            <p className="mt-1 text-xs text-slate-500">
              For Gmail, use smtp.gmail.com and an App Password. For Outlook, use smtp-mail.outlook.com.
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
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
                <label className="text-xs font-medium text-slate-600">SMTP Port</label>
                <input value={smtpForm.smtp_port} onChange={(e) => setSmtpForm({ ...smtpForm, smtp_port: e.target.value })}
                  placeholder="587"
                  className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
              </div>
              <div>
                <label className="text-xs font-medium text-slate-600">Username</label>
                <input value={smtpForm.username} onChange={(e) => setSmtpForm({ ...smtpForm, username: e.target.value })}
                  placeholder="you@company.com (usually same as email)"
                  className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
              </div>
              <div>
                <label className="text-xs font-medium text-slate-600">Password / App Password</label>
                <input type="password" value={smtpForm.password} onChange={(e) => setSmtpForm({ ...smtpForm, password: e.target.value })}
                  placeholder="••••••••"
                  className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-slate-500 focus:outline-none" />
              </div>
            </div>
            <div className="mt-4 flex gap-3">
              <button onClick={handleConnectSMTP} disabled={connectingEmail}
                className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50">
                {connectingEmail ? "Connecting..." : "Connect"}
              </button>
              <button onClick={() => setShowSMTPForm(false)}
                className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50">
                Cancel
              </button>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
