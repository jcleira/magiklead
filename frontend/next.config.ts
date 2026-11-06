import type { NextConfig } from "next";

// devpods sets DEVPOD_SUFFIX (default `localhost`): each devpod answers at
// `*.magiklead.localhost` and at `*.magiklead.<suffix>`, which another
// machine on the tailnet reaches. The dev server serves both.
const suffix = process.env.DEVPOD_SUFFIX?.trim();

const nextConfig: NextConfig = {
  allowedDevOrigins: [
    "mvp.magiklead.localhost",
    "*.magiklead.localhost",
    ...(suffix && suffix !== "localhost" ? [`*.magiklead.${suffix}`] : []),
  ],
};

export default nextConfig;
