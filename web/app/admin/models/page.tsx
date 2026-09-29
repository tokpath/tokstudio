"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { apiClient } from "@/lib/client";
import { type AdminModel, modelEditHref, vendorLabel } from "@/lib/catalog";
import { catalogStatusTone, modelStatusLabel } from "@/lib/catalog-admin";

type Route = { public_model_id: string; status: string; candidates?: unknown[] };

export default function AdminModelsPage() {
  const routes = useQuery({
    queryKey: ["/admin/routes"],
    queryFn: () => apiClient<{ items?: Route[] }>("GET", "/admin/routes"),
  });
  const byModel = new Map((routes.data?.items ?? []).map((route) => [route.public_model_id, route]));

  return (
    <AdminShell>
      <AdminListPanel<AdminModel>
        path="/admin/models"
        title="模型"
        emptyTitle="还没有模型"
        emptyDetail="创建模型时填写信息和售价，再发布配置。"
        rowHref={(row) => modelEditHref(row.id)}
        actions={
          <IfCan action="models.write">
            <Button asChild size="sm"><Link href="/admin/models/new">创建模型</Link></Button>
          </IfCan>
        }
        columns={[
          { accessorKey: "display_name", header: "模型名称" },
          { accessorKey: "id", header: "调用 ID", cell: ({ row }) => <span className="font-mono text-[13px]">{row.original.id}</span> },
          { accessorKey: "vendor", header: "原厂", cell: ({ row }) => vendorLabel(row.original.vendor) },
          { accessorKey: "status", header: "模型", cell: ({ row }) => <Badge tone={catalogStatusTone(row.original.status)}>{row.original.status === "published" && row.original.config_ready === false ? "待补配置" : modelStatusLabel(row.original.status)}</Badge> },
          {
            id: "route",
            header: "接入",
            cell: ({ row }) => {
              if (row.original.status !== "published") return "发布后可配置";
              if (row.original.config_ready === false) return "补齐类型和售价";
              const route = byModel.get(row.original.id);
              return route?.status === "active" && (route.candidates?.length ?? 0) > 0 ? "路由已启用" : "待配置路由";
            },
          },
        ]}
      />
    </AdminShell>
  );
}
