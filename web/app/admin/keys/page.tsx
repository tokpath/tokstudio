"use client";

import Link from "next/link";
import { AdminShell } from "../shell";

export default function AdminKeysPage() {
  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="mb-3 text-lg font-semibold tracking-tight">API Key</h2>
        <p className="text-sm text-ink-secondary">
          平台不再管理用户 API Key。渠道管理员请到{" "}
          <Link className="text-brand-emphasis underline-offset-4 hover:underline" href="/channel/keys">
            本渠道 API Key
          </Link>{" "}
          查看列表并禁用。终端用户在用户台创建与轮换自己的 Key。
        </p>
      </section>
    </AdminShell>
  );
}
