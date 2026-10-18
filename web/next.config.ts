import type { NextConfig } from "next";
const apiProxy = (process.env.TOKENHUB_API_INTERNAL_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
const nextConfig: NextConfig = {
  async headers() {
    return [
      { source: "/:path*", headers: [{ key: "Referrer-Policy", value: "strict-origin-when-cross-origin" }, { key: "X-Content-Type-Options", value: "nosniff" }] },
      ...["/staff-invitation", "/verify-email", "/reset-password"].map(source => ({ source, headers: [{ key: "Referrer-Policy", value: "no-referrer" }, { key: "Cache-Control", value: "no-store" }] })),
    ];
  },
  // Match Caddy handle_path: browser /api/v1/* reaches the backend at /v1/*.
  async rewrites() { return [{ source: "/api/:path*", destination: `${apiProxy}/:path*` }, { source: "/v1/:path*", destination: `${apiProxy}/v1/:path*` }]; },
};
export default nextConfig;
