import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Refund Policy",
  description:
    "MagikLead Refund Policy. 7-day refund window, credit consumption rules, and cancellation details.",
};

export default function RefundPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <h1 className="text-3xl font-bold tracking-tight text-slate-900">
        Refund Policy
      </h1>
      <p className="mt-2 text-sm text-slate-400">Last updated: April 10, 2026</p>

      <div className="mt-10 space-y-10 text-sm leading-relaxed text-slate-600">
        {/* 7-day window */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            7-Day Refund Window
          </h2>
          <p className="mt-3">
            If you are not satisfied with MagikLead, you may request a full
            refund within 7 days of your initial purchase or plan upgrade. To
            request a refund, email{" "}
            <a
              href="mailto:billing@magiklead.com"
              className="font-medium text-slate-900 underline"
            >
              billing@magiklead.com
            </a>{" "}
            with your account email and the reason for your request.
          </p>
        </section>

        {/* Lead credits */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Lead Credit Consumption
          </h2>
          <p className="mt-3">
            Refunds are not available if you have consumed more than 20% of your
            monthly lead quota during the refund window. Lead discovery is
            powered by paid third-party APIs, and consumed credits cannot be
            recovered.
          </p>
        </section>

        {/* Cancellation */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Cancellation
          </h2>
          <p className="mt-3">
            You may cancel your subscription at any time from the billing portal
            in your dashboard. Cancellation takes effect at the end of your
            current billing period — you will retain access to all paid features
            until then. No partial refunds are issued for unused time within a
            billing period.
          </p>
        </section>

        {/* Annual plans */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">
            Annual Plans
          </h2>
          <p className="mt-3">
            Annual subscriptions are eligible for a refund within the same 7-day
            window. After 7 days, annual plans are non-refundable. You may
            still cancel, and your access will continue until the end of the
            prepaid period.
          </p>
        </section>

        {/* Exceptions */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">Exceptions</h2>
          <p className="mt-3">
            We reserve the right to deny refund requests from accounts that show
            patterns of abuse, such as repeatedly subscribing and requesting
            refunds after consuming lead credits.
          </p>
        </section>

        {/* Contact */}
        <section>
          <h2 className="text-lg font-semibold text-slate-900">Contact</h2>
          <p className="mt-3">
            For billing questions, email{" "}
            <a
              href="mailto:billing@magiklead.com"
              className="font-medium text-slate-900 underline"
            >
              billing@magiklead.com
            </a>
            .
          </p>
        </section>
      </div>
    </div>
  );
}
