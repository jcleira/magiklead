import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: [
        "/dashboard",
        "/campaigns",
        "/settings",
        "/onboarding",
        "/leads",
      ],
    },
    sitemap: "https://magiklead.com/sitemap.xml",
  };
}
