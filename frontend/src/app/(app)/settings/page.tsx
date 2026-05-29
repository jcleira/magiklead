"use client";

import { useAuth, useClerk } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { useApi } from "@/hooks/use-api";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
const JWT_TEMPLATE = "magiklead-backend";

interface EmailAccount {
  id: string;
  email: string;
  provider: string;
  status: "connected" | "token_expired" | "scope_missing";
  last_polled_at: string | null;
}

interface Settings {
  workspace: { id: string; name: string };
  plan: { name: string; quota_leads_per_month: number };
  usage: { leads_saved_this_period: number };
  email_accounts: EmailAccount[];
}

const PLAN_LABEL: Record<string, string> = {
  free: "Free",
  starter: "Starter",
  growth: "Growth",
  scale: "Scale",
};

const UPGRADE_OPTIONS = [
  { name: "Starter", price: "$49/mo", leads: "500 leads", plan: "starter" },
  { name: "Growth", price: "$149/mo", leads: "2,000 leads", plan: "growth", popular: true },
  { name: "Scale", price: "$399/mo", leads: "10,000 leads", plan: "scale" },
];

export default function SettingsPage() {
  const { apiFetch } = useApi();
  const { getToken } = useAuth();
  const { signOut } = useClerk();
  const router = useRouter();
  const [settings, setSettings] = useState<Settings | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [upgrading, setUpgrading] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [disconnectTarget, setDisconnectTarget] = useState<EmailAccount | null>(null);
  const [disconnecting, setDisconnecting] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const loadSettings = useCallback(async () => {
    try {
      const data = await apiFetch<Settings>("/api/v1/settings");
      setSettings(data);
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : "Failed to load settings");
    }
  }, [apiFetch]);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  async function handleConnectGmail() {
    setConnecting(true);
    try {
      const { url } = await apiFetch<{ url: string }>("/api/v1/gmail/auth-url");
      window.location.href = url;
    } catch (e) {
      alert("Failed to start Gmail connect: " + (e instanceof Error ? e.message : "unknown error"));
      setConnecting(false);
    }
  }

  async function handleConfirmDisconnect() {
    if (!disconnectTarget) return;
    setDisconnecting(true);
    try {
      await apiFetch(`/api/v1/gmail/accounts/${disconnectTarget.id}`, { method: "DELETE" });
      setDisconnectTarget(null);
      await loadSettings();
    } catch (e) {
      alert("Failed to disconnect: " + (e instanceof Error ? e.message : "unknown error"));
    } finally {
      setDisconnecting(false);
    }
  }

  async function handleUpgrade(plan: string) {
    setUpgrading(true);
    try {
      const { url } = await apiFetch<{ url: string }>("/api/v1/billing/checkout", {
        method: "POST",
        body: JSON.stringify({ plan }),
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
      const { url } = await apiFetch<{ url: string }>("/api/v1/billing/portal", { method: "POST" });
      window.location.href = url;
    } catch {
      alert("No active subscription to manage.");
    }
  }

  // apiFetch always parses JSON; the export endpoint streams a zip, so
  // we use a raw fetch here with the same Clerk-JWT auth pattern.
  async function handleExport() {
    setExporting(true);
    try {
      const token = await getToken({ template: JWT_TEMPLATE });
      if (!token) throw new Error("Not authenticated");
      const res = await fetch(`${API_URL}/api/v1/account/export`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.message || `Export failed: ${res.status}`);
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `magiklead-export-${settings?.workspace.id ?? "data"}.zip`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (e) {
      alert("Failed to export data: " + (e instanceof Error ? e.message : "unknown error"));
    } finally {
      setExporting(false);
    }
  }

  async function handleConfirmDelete() {
    setDeleteError(null);
    setDeleting(true);
    try {
      await apiFetch("/api/v1/account", { method: "DELETE" });
      // Local data is gone; sign the user out of Clerk and bounce
      // them back to the marketing landing page.
      await signOut();
      router.replace("/");
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : "Delete failed. Try again.");
      setDeleting(false);
    }
  }

  if (loadError && !settings) {
    return (
      <div className="p-8">
        <h1 className="text-2xl font-bold text-slate-900">Settings</h1>
        <p className="mt-4 text-sm text-red-600">{loadError}</p>
      </div>
    );
  }

  if (!settings) {
    return (
      <div className="p-8">
        <h1 className="text-2xl font-bold text-slate-900">Settings</h1>
        <p className="mt-4 text-sm text-slate-500">Loading…</p>
      </div>
    );
  }

  const planLabel = PLAN_LABEL[settings.plan.name] ?? settings.plan.name;
  const leadsUsed = settings.usage.leads_saved_this_period;
  const leadsQuota = settings.plan.quota_leads_per_month;
  const usagePct = leadsQuota > 0 ? Math.min(100, Math.round((leadsUsed / leadsQuota) * 100)) : 0;

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold text-slate-900">Settings</h1>

      {/* Workspace */}
      <section className="mt-8">
        <h2 className="text-lg font-semibold text-slate-900">Workspace</h2>
        <div className="mt-3 rounded-xl border border-slate-200 p-6">
          <p className="text-sm text-slate-500">Name</p>
          <p className="mt-1 text-xl font-semibold text-slate-900">{settings.workspace.name}</p>
        </div>
      </section>

      {/* Billing */}
      <section className="mt-10">
        <h2 className="text-lg font-semibold text-slate-900">Billing</h2>
        <div className="mt-4 rounded-xl border border-slate-200 p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-500">Current plan</p>
              <p className="mt-1 text-xl font-bold text-slate-900">{planLabel}</p>
              <p className="mt-1 text-sm text-slate-400">
                {leadsQuota.toLocaleString()} leads/mo
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
            <div className="mt-3">
              <div className="flex justify-between text-sm">
                <span className="text-slate-500">Leads saved</span>
                <span className="font-medium text-slate-900">
                  {leadsUsed.toLocaleString()} / {leadsQuota.toLocaleString()}
                </span>
              </div>
              <div className="mt-1.5 h-2 rounded-full bg-slate-100">
                <div className="h-2 rounded-full bg-slate-900" style={{ width: `${usagePct}%` }} />
              </div>
            </div>
          </div>

          {settings.plan.name === "free" && (
            <div className="mt-6 border-t border-slate-200 pt-6">
              <p className="text-sm font-medium text-slate-700">Upgrade</p>
              <div className="mt-3 grid gap-3 sm:grid-cols-3">
                {UPGRADE_OPTIONS.map((option) => (
                  <button
                    key={option.plan}
                    onClick={() => handleUpgrade(option.plan)}
                    disabled={upgrading}
                    className={`rounded-lg border p-4 text-left transition-colors hover:border-slate-400 disabled:opacity-50 ${
                      option.popular ? "border-slate-900" : "border-slate-200"
                    }`}
                  >
                    <p className="text-sm font-semibold text-slate-900">{option.name}</p>
                    <p className="mt-0.5 text-lg font-bold text-slate-900">{option.price}</p>
                    <p className="mt-1 text-xs text-slate-500">{option.leads}</p>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </section>

      {/* Email Accounts */}
      <section className="mt-10">
        <h2 className="text-lg font-semibold text-slate-900">Email Accounts</h2>
        <p className="mt-1 text-sm text-slate-500">
          Connect your Gmail to send from your own address and detect replies.
        </p>

        <div className="mt-4 space-y-3">
          {settings.email_accounts.map((acc) => (
            <EmailAccountRow
              key={acc.id}
              account={acc}
              onReconnect={handleConnectGmail}
              onDisconnect={() => setDisconnectTarget(acc)}
            />
          ))}
        </div>

        <button
          onClick={handleConnectGmail}
          disabled={connecting}
          className="mt-4 rounded-lg bg-slate-900 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
        >
          {connecting ? "Redirecting…" : "Connect Gmail"}
        </button>
      </section>

      {/* Export data */}
      <section className="mt-10">
        <h2 className="text-lg font-semibold text-slate-900">Export data</h2>
        <p className="mt-1 text-sm text-slate-500">
          Download an archive of your campaigns, sequences, saved leads,
          sends, and replies. The download is a single .zip containing one
          JSON file per data type. Gmail OAuth tokens are redacted.
        </p>
        <button
          onClick={handleExport}
          disabled={exporting}
          className="mt-4 rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50"
        >
          {exporting ? "Preparing archive…" : "Download"}
        </button>
      </section>

      {/* Delete account */}
      <section className="mt-10 border-t border-red-100 pt-10">
        <h2 className="text-lg font-semibold text-red-700">Delete account</h2>
        <p className="mt-1 text-sm text-slate-500">
          Permanently erase your workspace, saved leads, campaigns, sends,
          replies, and Gmail connection. Your Clerk login is removed too.
          This cannot be undone.
        </p>
        <button
          onClick={() => {
            setDeleteError(null);
            setDeleteConfirmText("");
            setShowDeleteModal(true);
          }}
          className="mt-4 rounded-lg border border-red-300 px-4 py-2 text-sm font-semibold text-red-700 hover:bg-red-50"
        >
          Delete account
        </button>
      </section>

      {disconnectTarget && (
        <DisconnectModal
          account={disconnectTarget}
          working={disconnecting}
          onCancel={() => setDisconnectTarget(null)}
          onConfirm={handleConfirmDisconnect}
        />
      )}

      {showDeleteModal && (
        <DeleteAccountModal
          workspaceName={settings.workspace.name}
          confirmText={deleteConfirmText}
          onConfirmTextChange={setDeleteConfirmText}
          error={deleteError}
          working={deleting}
          onCancel={() => setShowDeleteModal(false)}
          onConfirm={handleConfirmDelete}
        />
      )}
    </div>
  );
}

function DeleteAccountModal({
  workspaceName,
  confirmText,
  onConfirmTextChange,
  error,
  working,
  onCancel,
  onConfirm,
}: {
  workspaceName: string;
  confirmText: string;
  onConfirmTextChange: (s: string) => void;
  error: string | null;
  working: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  // The destructive action requires typing the workspace name
  // verbatim — guards against accidental clicks on a deeply
  // unrecoverable operation. Case-sensitive match.
  const armed = confirmText === workspaceName && !working;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4">
      <div className="w-full max-w-md rounded-xl bg-white p-6 shadow-xl">
        <h3 className="text-base font-semibold text-red-700">Delete this workspace?</h3>
        <p className="mt-2 text-sm text-slate-600">
          Every campaign, saved lead, send record, and Gmail connection in{" "}
          <span className="font-semibold">{workspaceName}</span> will be
          erased. Your Clerk login is removed. No grace period.
        </p>
        <p className="mt-4 text-sm text-slate-700">
          To confirm, type{" "}
          <span className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-slate-800">
            {workspaceName}
          </span>{" "}
          below:
        </p>
        <input
          type="text"
          value={confirmText}
          onChange={(e) => onConfirmTextChange(e.target.value)}
          disabled={working}
          className="mt-2 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-red-400 focus:outline-none focus:ring-1 focus:ring-red-400 disabled:opacity-50"
          placeholder={workspaceName}
        />
        {error && <p className="mt-3 text-sm text-red-600">{error}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <button
            onClick={onCancel}
            disabled={working}
            className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={!armed}
            className="rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50"
          >
            {working ? "Deleting…" : "Delete forever"}
          </button>
        </div>
      </div>
    </div>
  );
}

function EmailAccountRow({
  account,
  onReconnect,
  onDisconnect,
}: {
  account: EmailAccount;
  onReconnect: () => void;
  onDisconnect: () => void;
}) {
  const indicator = STATUS_INDICATOR[account.status] ?? STATUS_INDICATOR.connected;
  const needsAction = account.status !== "connected";
  return (
    <div className="flex items-center justify-between rounded-xl border border-slate-200 p-4">
      <div className="flex items-center gap-3">
        <span
          aria-hidden="true"
          className={`h-2.5 w-2.5 rounded-full ${indicator.dotClass}`}
        />
        <div>
          <p className="text-sm font-medium text-slate-900">{account.email}</p>
          <p className="text-xs text-slate-400">
            {account.provider.toUpperCase()} · {indicator.label}
            {account.last_polled_at ? ` · last poll ${formatRelative(account.last_polled_at)}` : ""}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-3">
        {needsAction && (
          <button
            onClick={onReconnect}
            className="rounded-lg border border-slate-300 px-3 py-1.5 text-sm font-semibold text-slate-700 hover:bg-slate-50"
          >
            {indicator.cta}
          </button>
        )}
        <button
          onClick={onDisconnect}
          className="text-sm text-red-500 hover:text-red-700"
        >
          Disconnect
        </button>
      </div>
    </div>
  );
}

function DisconnectModal({
  account,
  working,
  onCancel,
  onConfirm,
}: {
  account: EmailAccount;
  working: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4">
      <div className="w-full max-w-md rounded-xl bg-white p-6 shadow-xl">
        <h3 className="text-base font-semibold text-slate-900">Disconnect Gmail account</h3>
        <p className="mt-2 text-sm text-slate-600">
          {account.email} will no longer send for any campaign. Existing sends remain
          in your history; in-flight sequences will stop on this mailbox.
        </p>
        <div className="mt-5 flex justify-end gap-2">
          <button
            onClick={onCancel}
            disabled={working}
            className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={working}
            className="rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50"
          >
            {working ? "Disconnecting…" : "Disconnect"}
          </button>
        </div>
      </div>
    </div>
  );
}

const STATUS_INDICATOR: Record<
  EmailAccount["status"],
  { label: string; dotClass: string; cta: string }
> = {
  connected: { label: "Connected", dotClass: "bg-green-500", cta: "Reconnect" },
  token_expired: { label: "Token expired", dotClass: "bg-amber-500", cta: "Reconnect" },
  scope_missing: { label: "Scope missing", dotClass: "bg-red-500", cta: "Re-authorise" },
};

function formatRelative(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const diffSec = Math.round((Date.now() - t) / 1000);
  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffSec < 3600) return `${Math.round(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.round(diffSec / 3600)}h ago`;
  return `${Math.round(diffSec / 86400)}d ago`;
}
