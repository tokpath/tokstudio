"use client";

import { useQuery } from "@tanstack/react-query";
import { AdminShell } from "../shell";
import { apiClient } from "@/lib/client";

export default function AdminBillingPage() {
  const query = useQuery({
    queryKey: ["billing-report"],
    queryFn: () => apiClient<{ report?: Record<string, number>; error?: { message?: string } }>("GET", "/admin/billing/report"),
  });
  const report = query.data?.report || {};
  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">余额 / 充值 / 账务</h2>
        <p className="text-sm text-slate-400">退款和手工入账必须带 X-Tokenhub-Confirm: 1；启用 2FA 后再带 TOTP。</p>
        <pre className="mt-3 overflow-x-auto text-sm text-slate-200">{JSON.stringify(report, null, 2) || query.data?.error?.message}</pre>
      </section>
    </AdminShell>
  );
}
