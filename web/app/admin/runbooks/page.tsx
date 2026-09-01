"use client";

import { useQuery } from "@tanstack/react-query";
import { AdminShell } from "../shell";
import { apiClient } from "@/lib/client";
import { AdminH2 } from "@/components/admin-h2";

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
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="runbooks" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">每种关键告警对应一份处置步骤。对账、熔断、备份和 TLS 演练都有手册。</p>
        {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        <div className="grid gap-3">
          {items.map((item) => (
            <article key={item.id} className="rounded-xl border border-hairline bg-canvas-raised p-4">
              <p className="text-xs uppercase tracking-wide text-brand-emphasis">{item.alert_kind}</p>
              <h3 className="mt-1 text-lg font-medium text-ink">{item.title}</h3>
              <p className="mt-2 whitespace-pre-wrap text-sm text-ink-secondary">{item.body}</p>
            </article>
          ))}
        </div>
      </section>
    </AdminShell>
  );
}
