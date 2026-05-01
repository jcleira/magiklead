import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Privacy Policy",
  description:
    "MagikLead Privacy Policy. What data we collect — from customers and from data subjects in our canonical lead database — how we use it, and how to exercise your rights under GDPR and CCPA.",
};

export default function PrivacyPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <h1 className="text-3xl font-bold tracking-tight text-slate-900">
        Privacy Policy
      </h1>
      <p className="mt-2 text-sm text-slate-400">Last updated: April 24, 2026</p>

      <div className="mt-10 space-y-10 text-sm leading-relaxed text-slate-600">
        <p>
          This policy covers two categories of individuals: our{" "}
          <strong>customers</strong> (people who sign up for MagikLead
          accounts) and <strong>data subjects in our canonical lead
          database</strong> (individuals whose professional information
          we have collected from public sources so that our customers
          can search for them). Your rights and the data we hold differ
          depending on which category applies to you; both are described
          below.
        </p>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Customer Data — What We Collect
          </h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              <strong>Account information</strong> — name, email, and
              authentication data provided through Clerk.
            </li>
            <li>
              <strong>Payment information</strong> — billing details
              processed and stored by Stripe. We never see or store your
              full card number.
            </li>
            <li>
              <strong>Usage data</strong> — campaigns created, leads
              saved, sequences generated, and feature usage for
              analytics and billing.
            </li>
            <li>
              <strong>Connected accounts</strong> — OAuth tokens for
              Gmail that grant the send-only scope (
              <code>gmail.send</code>); we cannot read your inbox.
            </li>
            <li>
              <strong>Device and log data</strong> — IP address, browser
              type, and access timestamps collected automatically.
            </li>
          </ul>
        </section>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Canonical Lead Database — What We Collect
          </h2>
          <p className="mt-3">
            We build and maintain a canonical database of professional
            contact information — names, current titles, employer,
            publicly published work email addresses, and links to
            publicly published profiles — drawn from public sources
            including SEC EDGAR filings, Wikidata, and CrunchBase. We
            deduplicate and resolve records into canonical persons so
            our customers can search across the graph without creating
            duplicates.
          </p>
          <p className="mt-3">
            We do not collect special-category data (health, religion,
            sexual orientation, etc.). We do not knowingly collect data
            about minors.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            How We Use It
          </h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>To provide, maintain, and improve MagikLead.</li>
            <li>To process payments and manage subscriptions.</li>
            <li>
              To send transactional emails (account confirmations,
              billing receipts, erasure confirmations).
            </li>
            <li>
              To generate AI-personalized outreach sequences using the
              business context the customer provides.
            </li>
            <li>To detect abuse and enforce our Terms of Service.</li>
          </ul>
        </section>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Third-Party Services
          </h2>
          <p className="mt-3">
            We share data only with the services required to operate
            MagikLead:
          </p>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              <strong>Clerk</strong> — authentication and user
              management.
            </li>
            <li>
              <strong>Stripe</strong> — payment processing and
              subscription billing.
            </li>
            <li>
              <strong>Anthropic</strong> — AI sequence generation. We
              send the customer&apos;s business profile and the target
              lead&apos;s public professional details to generate the
              message. No inbox content is sent.
            </li>
            <li>
              <strong>Google (Gmail API)</strong> — delivering outbound
              emails the customer authors, using the send-only scope.
            </li>
          </ul>
          <p className="mt-3">
            We do not sell your personal data to any third party.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Your Rights — Data Subjects in the Canonical Database
          </h2>
          <p className="mt-3">
            If we have a record about you and you did not sign up for
            MagikLead, you can request erasure at{" "}
            <Link
              href="/privacy/erasure"
              className="font-medium text-slate-900 underline"
            >
              /privacy/erasure
            </Link>
            . We&apos;ll email you a confirmation link that expires in
            24 hours. After you confirm, we:
          </p>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              Hard-delete the canonical record and everything linked to
              it (employments, emails, phone numbers, aliases, identifiers).
            </li>
            <li>
              Add a one-way hash of your strong identifiers (email,
              LinkedIn URL) to an ingest blocklist so subsequent data
              refreshes do not recreate the record.
            </li>
          </ul>
          <p className="mt-3">
            We do not retain the raw identifiers — only the hash — so
            you can stay erased without us keeping your details on file.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            GDPR Rights (EEA Users)
          </h2>
          <p className="mt-3">
            If you are in the European Economic Area, you have the right
            to:
          </p>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>Access the personal data we hold about you.</li>
            <li>Request correction of inaccurate data.</li>
            <li>
              Request deletion of your data. For canonical database
              records this is self-serve at{" "}
              <Link
                href="/privacy/erasure"
                className="font-medium text-slate-900 underline"
              >
                /privacy/erasure
              </Link>
              .
            </li>
            <li>Object to or restrict processing of your data.</li>
            <li>Request data portability.</li>
          </ul>
          <p className="mt-3">
            For any request beyond self-serve erasure, email us at the
            address below.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            CCPA Rights (California Residents)
          </h2>
          <p className="mt-3">
            California residents may request disclosure of the
            categories and specific pieces of personal information
            collected, request deletion, and opt out of the sale of
            personal information. We do not sell personal information.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Data Retention
          </h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              <strong>Customer account data</strong> is retained while
              your account is active. On account deletion we remove
              personal data within 30 days, except records we are
              required by law to retain (e.g., billing records for tax
              purposes).
            </li>
            <li>
              <strong>Canonical database records</strong> are retained
              until the data subject requests erasure, the source
              material changes so the record is no longer supported, or
              we remove the source entirely. Blocklist hashes are
              retained indefinitely so erasure persists across future
              refreshes.
            </li>
          </ul>
        </section>

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
