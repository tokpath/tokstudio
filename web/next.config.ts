import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin("./i18n/request.ts");

const apiProxy = (process.env.TOKENHUB_API_INTERNAL_URL || "http://127.0.0.1:8080").replace(/\/$/, "");

const nextConfig: NextConfig = {
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiProxy}/:path*` }];
  },
};

export default withNextIntl(nextConfig);
