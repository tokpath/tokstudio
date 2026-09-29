"use client";

import Link from "next/link";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { catalogStatusTone, routeStatusLabel, routeStrategyLabel } from "@/lib/catalog-admin";

type Route = {
  id: string; public_model_id: string; strategy: string; status: string;
  candidates?: { provider_slug?: string; provider_id?: string }[];
};

export default function AdminRoutesPage() {
  return <AdminShell>
    <AdminListPanel<Route>
      path="/admin/routes"
      title="路由组"
      emptyTitle="还没有路由组"
      emptyDetail="先发布模型，再配置提供商和上游模型标识。"
      rowHref={(row) => `/admin/routes/${encodeURIComponent(row.id)}`}
      actions={<IfCan action="routes.write"><Button asChild size="sm"><Link href="/admin/routes/new">创建路由组</Link></Button></IfCan>}
      columns={[
        { accessorKey: "public_model_id", header: "公开模型" },
        { accessorKey: "strategy", header: "策略", cell: ({ row }) => routeStrategyLabel(row.original.strategy) },
        { id: "providers", header: "提供商", cell: ({ row }) => row.original.candidates?.map((item) => item.provider_slug || item.provider_id).join(" → ") || "尚未配置" },
        { accessorKey: "status", header: "状态", cell: ({ row }) => <Badge tone={catalogStatusTone(row.original.status)}>{routeStatusLabel(row.original.status)}</Badge> },
      ]}
    />
  </AdminShell>;
}
