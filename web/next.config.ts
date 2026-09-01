import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin("./i18n/request.ts");

const apiProxy = (process.env.TOKENHUB_API_INTERNAL_URL || "http://127.0.0.1:8080").replace(/\/$/, "");

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiProxy}/:path*` },
      // 磁盘目录不能叫 app/app：next build 会把 app/page.tsx 和 app/app/page.tsx
      // 编成同一份模块，生产包的 / 会变成用户台。URL 仍对外暴露 /app。
      { source: "/app", destination: "/console" },
      { source: "/app/:path*", destination: "/console/:path*" },
    ];
  },
};

export default withNextIntl(nextConfig);
