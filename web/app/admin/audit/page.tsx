"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type Audit = { id: string; action: string; resource_type: string; resource_id: string };

export default function AdminAuditPage() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("Outbox 看 pending/published/failed。探测只在沙箱写一条 audit.probe，生产会 403。");

  return (
    <AdminShell>
      <section className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
        <h2 className="mb-3 text-xl font-medium">Outbox 与探测</h2>
        <p className="mb-3 text-sm text-slate-400">
          pending 太高说明 worker 没跟上。探测用来确认审计链路还能写，不会计费，也不要二次确认。
        </p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/outbox/stats`, { credentials: "include" });
              const body = await res.json();
              setMessage(
                res.ok
                  ? `Outbox pending=${body.stats?.pending ?? 0} published=${body.stats?.published ?? 0} failed=${body.stats?.failed ?? 0}`
                  : body.error?.message || "读取失败",
              );
            }}
          >
            读取 Outbox
          </Button>
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
                setMessage(body.error?.message || "探测失败");
                return;
              }
              setMessage(`已写入探测 ${body.item?.id} → ${body.item?.action}`);
              await queryClient.invalidateQueries();
            }}
          >
            写入探测
          </Button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
      <AdminListPanel<Audit>
        path="/admin/audit-logs"
        title="审计日志"
        columns={[
          { accessorKey: "action", header: "Action" },
          { accessorKey: "resource_type", header: "Resource" },
          { accessorKey: "resource_id", header: "ID" },
        ]}
      />
    </AdminShell>
  );
}
