"use client";

import { useLocale } from "next-intl";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type Audit = { id: string; action: string; resource_type: string; resource_id: string; actor_user_id?: string; created_at?: string; before?: unknown; after?: unknown; request_id?: string };

export default function AdminAuditPage() {
  const displayLocale = useLocale();
  const tUI = useTranslations();
  const queryClient = useQueryClient();
  const [message, setMessage] = useState(tUI("consoleCopy.outboxShowsPendingPublishedAndFailedEventsProbes"));

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 level={1} k="audit" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">
          {tUI("consoleCopy.aLargePendingQueueMayIndicateAWorker")}</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <IfCan action="audit.outbox">
            <Button
              size="sm"
              variant="outline"
              onClick={async () => {
                const res = await fetch(`${apiBase}/admin/outbox/stats`, { credentials: "include" });
                const body = await res.json();
                setMessage(
                  res.ok
                    ? `Outbox pending=${body.stats?.pending ?? 0} published=${body.stats?.published ?? 0} failed=${body.stats?.failed ?? 0}`
                    : body.error?.message || tUI("workbench.readFailed"),
                );
              }}
            >
              {tUI("consoleCopy.readOutbox")}</Button>
          </IfCan>
          <IfCan action="audit.probe">
            <Button
              size="sm"
              onClick={async () => {
                const res = await fetch(`${apiBase}/admin/audit-probes`, {
                  method: "POST",
                  credentials: "include",
                  headers: { "Content-Type": "application/json" },
                  body: "{}",
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || tUI("consoleCopy.probeFailed"));
                  return;
                }
                setMessage(tUI("consoleCopy.probeRecorded", { value0: String(body.item?.id), value1: String(body.item?.action) }));
                await queryClient.invalidateQueries();
              }}
            >
              {tUI("consoleCopy.writeProbe")}</Button>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <AdminListPanel<Audit>
        path="/admin/audit-logs"
        title={tUI("admin.audit")}
        columns={[
          { accessorKey: "created_at", header: tUI("adminUi.colTime"), cell: ({row}) => row.original.created_at ? new Date(row.original.created_at).toLocaleString(displayLocale) : "—" },
          { accessorKey: "actor_user_id", header: tUI("consoleCopy.actorId"), cell: ({row}) => row.original.actor_user_id || tUI("theme.system") },
          { accessorKey: "action", header: tUI("user.colActions") },
          { accessorKey: "resource_type", header: tUI("oem.resourceType") },
          { accessorKey: "resource_id", header: tUI("oem.resource") },
          { id: "details", header: tUI("consoleCopy.changeEvidence"), cell: ({row}) => <details><summary className="cursor-pointer text-brand-emphasis">{tUI("user.viewDetails")}</summary><p className="mt-2">{tUI("consoleCopy.requestId")}{row.original.request_id || "—"}</p><p className="mt-2 font-medium">{tUI("consoleCopy.before")}</p><pre className="max-w-md overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(row.original.before ?? {},null,2)}</pre><p className="mt-2 font-medium">{tUI("consoleCopy.afterIncludingTheReason")}</p><pre className="max-w-md overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(row.original.after ?? {},null,2)}</pre></details> },
        ]}
      />
    </AdminShell>
  );
}
