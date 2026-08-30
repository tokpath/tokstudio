"use client";

import { useQuery } from "@tanstack/react-query";
import { AdminShell } from "../shell";
import { apiClient } from "@/lib/client";

type Runbook = { id: string; alert_kind: string; title: string; body: string };
type ListResponse = { items?: Runbook[]; error?: { message?: string } };

export default function AdminRunbooksPage() {
  const query = useQuery({
    queryKey: ["/admin/ops/runbooks"],
    queryFn: () => apiClient<ListResponse>("GET", "/admin/ops/runbooks"),
  });
  const items = query.data?.items ?? [];

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">应急手册</h2>
        <p className="mb-3 text-sm text-slate-400">每种关键告警对应一份处置步骤。对账、熔断、备份和 TLS 演练都有手册。</p>
        {query.data?.error ? <p className="text-sm text-slate-400">{query.data.error.message}</p> : null}
        <div className="grid gap-3">
          {items.map((item) => (
            <article key={item.id} className="rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <p className="text-xs uppercase tracking-wide text-cyan-300">{item.alert_kind}</p>
              <h3 className="mt-1 text-lg font-medium text-slate-100">{item.title}</h3>
              <p className="mt-2 whitespace-pre-wrap text-sm text-slate-300">{item.body}</p>
            </article>
          ))}
        </div>
      </section>
    </AdminShell>
  );
}
