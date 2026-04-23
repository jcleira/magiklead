import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Privacy Policy",
  description:
    "MagikLead Privacy Policy. Learn what data we collect, how we use it, and your rights under GDPR and CCPA.",
};

export default function PrivacyPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <h1 className="text-3xl font-bold tracking-tight text-slate-900">
        Privacy Policy
      </h1>
      <p className="mt-2 text-sm text-slate-400">Last updated: April 10, 2026</p>

      <div className="mt-10 space-y-10 text-sm leading-relaxed text-slate-600">
        {/* What We Collect */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            What We Collect
          </h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              <strong>Account information</strong> — name, email address, and
              authentication data provided through Clerk.
            </li>
            <li>
              <strong>Payment information</strong> — billing details processed
              and stored by Stripe. We never see or store your full card number.
            </li>
            <li>
              <strong>Usage data</strong> — campaigns created, leads discovered,
              sequences sent, and feature usage for analytics and billing.
            </li>
            <li>
              <strong>Connected accounts</strong> — OAuth tokens for Gmail to
              send emails on your behalf. We do not read or store your inbox
              contents.
            </li>
            <li>
              <strong>Device and log data</strong> — IP address, browser type,
              and access timestamps collected automatically.
            </li>
          </ul>
        </section>

        {/* How We Use It */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            How We Use It
          </h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>To provide, maintain, and improve the MagikLead service.</li>
            <li>To process payments and manage subscriptions.</li>
            <li>
              To send transactional emails (account confirmations, billing
              receipts, service updates).
            </li>
            <li>
              To generate AI-personalized outreach sequences using the context
              you provide.
            </li>
            <li>To detect abuse and enforce our Terms of Service.</li>
          </ul>
        </section>

        {/* Third Parties */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Third-Party Services
          </h2>
          <p className="mt-3">
            We share data only with the services required to operate MagikLead:
          </p>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              <strong>Clerk</strong> — authentication and user management.
            </li>
            <li>
              <strong>Stripe</strong> — payment processing and subscription
              billing.
            </li>
            <li>
              <strong>Anthropic</strong> — AI sequence generation. We send
              limited context (your product info and lead profile) to generate
              personalized messages. No email content or inbox data is shared.
            </li>
            <li>
              <strong>RapidAPI</strong> — lead discovery and email
              verification services.
            </li>
          </ul>
          <p className="mt-3">
            We do not sell your personal data to any third party.
          </p>
        </section>

        {/* GDPR */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            GDPR Rights (EEA Users)
          </h2>
          <p className="mt-3">
            If you are in the European Economic Area, you have the right to:
          </p>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>Access the personal data we hold about you.</li>
            <li>Request correction of inaccurate data.</li>
            <li>Request deletion of your data.</li>
            <li>Object to or restrict processing of your data.</li>
            <li>Request data portability.</li>
          </ul>
          <p className="mt-3">
            To exercise any of these rights, email us at the address below.
          </p>
        </section>

        {/* CCPA */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            CCPA Rights (California Residents)
          </h2>
          <p className="mt-3">
            California residents may request disclosure of the categories and
            specific pieces of personal information collected, request deletion,
            and opt out of the sale of personal information. We do not sell
            personal information.
          </p>
        </section>

        {/* Data Retention */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Data Retention
          </h2>
          <p className="mt-3">
            We retain your account data for as long as your account is active.
            If you delete your account, we remove your personal data within 30
            days, except where we are required by law to retain it (e.g.,
            billing records for tax purposes). Cached lead data is retained for
            up to 90 days to prevent duplicate charges.
          </p>
        </section>

        {/* Contact */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">Contact</h2>
          <p className="mt-3">
            For privacy-related questions or requests, email us at{" "}
            <a
              href="mailto:privacy@magiklead.com"
              className="font-medium text-slate-900 underline"
            >
              privacy@magiklead.com
            </a>
            .
          </p>
        </section>
      </div>
    </div>
  );
}
