import type { Metadata } from "next";
import { Geist, Geist_Mono, Instrument_Serif } from "next/font/google";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

const instrumentSerif = Instrument_Serif({
  variable: "--font-display",
  subsets: ["latin"],
  weight: "400",
});

export const metadata: Metadata = {
  title: {
    default: "MagikLead — AI Outreach That Doesn't Cost a Fortune",
    template: "%s | MagikLead",
  },
  description:
    "Paste your website. Get leads. Send personalized emails and LinkedIn messages. Starting at $49/mo — 50% cheaper than the competition.",
  metadataBase: new URL("https://magiklead.com"),
  openGraph: {
    title: "MagikLead — AI Outreach That Doesn't Cost a Fortune",
    description:
      "Paste your website. Get leads. Send personalized emails and LinkedIn messages. Starting at $49/mo.",
    siteName: "MagikLead",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "MagikLead — AI Outreach That Doesn't Cost a Fortune",
    description:
      "Paste your website. Get leads. Send personalized emails. Starting at $49/mo.",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} ${instrumentSerif.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col" suppressHydrationWarning>
        <script dangerouslySetInnerHTML={{ __html: `
          (function() {
            var t = localStorage.getItem('theme');
            if (t === 'dark' || (!t && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
              document.documentElement.classList.add('dark');
            }
          })();
        ` }} />
        {children}
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              "@context": "https://schema.org",
              "@type": "SoftwareApplication",
              name: "MagikLead",
              description:
                "AI-powered cold email and outreach automation platform",
              applicationCategory: "BusinessApplication",
              offers: {
                "@type": "Offer",
                price: "0",
                priceCurrency: "USD",
              },
              url: "https://magiklead.com",
            }),
          }}
        />
      </body>
    </html>
  );
}
