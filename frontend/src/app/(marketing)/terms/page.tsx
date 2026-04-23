import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Terms of Service",
  description:
    "MagikLead Terms of Service. Acceptable use, account responsibility, service limitations, and liability.",
};

export default function TermsPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <h1 className="text-3xl font-bold tracking-tight text-slate-900">
        Terms of Service
      </h1>
      <p className="mt-2 text-sm text-slate-400">Last updated: April 10, 2026</p>

      <div className="mt-10 space-y-10 text-sm leading-relaxed text-slate-600">
        <p>
          By using MagikLead (&quot;the Service&quot;), you agree to the
          following terms. If you do not agree, do not use the Service.
        </p>

        {/* Acceptable Use */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Acceptable Use
          </h2>
          <p className="mt-3">You agree to:</p>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              Comply with the CAN-SPAM Act, GDPR, and all applicable email and
              privacy laws in your jurisdiction.
            </li>
            <li>
              Only contact prospects who have not explicitly opted out of
              receiving outreach.
            </li>
            <li>
              Include a working unsubscribe mechanism in every outbound email.
            </li>
            <li>
              Not use MagikLead to send spam, phishing, malware, or any
              fraudulent content.
            </li>
            <li>
              Not scrape, resell, or redistribute lead data obtained through the
              Service.
            </li>
          </ul>
          <p className="mt-3">
            We reserve the right to suspend or terminate accounts that violate
            these rules without notice or refund.
          </p>
        </section>

        {/* Account Responsibility */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Account Responsibility
          </h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              You are responsible for maintaining the security of your account
              credentials.
            </li>
            <li>
              You are responsible for all activity that occurs under your
              account.
            </li>
            <li>
              You must be at least 18 years old to use the Service.
            </li>
            <li>
              You agree to provide accurate billing and account information.
            </li>
          </ul>
        </section>

        {/* Service Limitations */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Service Limitations
          </h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              Lead discovery depends on third-party data providers. We do not
              guarantee the accuracy, completeness, or deliverability of any
              lead data.
            </li>
            <li>
              Email sending is subject to your Gmail account limits and Google
              Terms of Service. MagikLead enforces daily send limits to protect
              your account.
            </li>
            <li>
              AI-generated content is provided as a starting point. You are
              responsible for reviewing and approving all outbound messages.
            </li>
            <li>
              We may modify, suspend, or discontinue features with reasonable
              notice.
            </li>
          </ul>
        </section>

        {/* Liability Limitation */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Limitation of Liability
          </h2>
          <p className="mt-3">
            The Service is provided &quot;as is&quot; without warranties of any
            kind, express or implied. To the maximum extent permitted by law,
            MagikLead shall not be liable for any indirect, incidental, special,
            or consequential damages arising from your use of the Service,
            including but not limited to lost revenue, data loss, or account
            suspension by third-party platforms.
          </p>
          <p className="mt-3">
            Our total liability for any claim related to the Service is limited
            to the amount you paid us in the 12 months preceding the claim.
          </p>
        </section>

        {/* Termination */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">Termination</h2>
          <ul className="mt-3 list-disc space-y-1 pl-5">
            <li>
              You may cancel your account at any time from the billing portal.
              Cancellation takes effect at the end of the current billing
              period.
            </li>
            <li>
              We may terminate your account immediately if you violate these
              Terms, with or without notice.
            </li>
            <li>
              Upon termination, your data will be deleted in accordance with our
              Privacy Policy.
            </li>
          </ul>
        </section>

        {/* Contact */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">Contact</h2>
          <p className="mt-3">
            Questions about these terms? Email us at{" "}
            <a
              href="mailto:legal@magiklead.com"
              className="font-medium text-slate-900 underline"
            >
              legal@magiklead.com
            </a>
            .
          </p>
        </section>
      </div>
    </div>
  );
}
