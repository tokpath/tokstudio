"use client";

import { useState } from "react";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { CreateProviderDialog } from "./create-dialog";
import { ProbeCell } from "./probe-cell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import {
  adapterLabel,
  healthLabel,
  healthTone,
  providerHref,
  providerKindLabel,
  providerStatusLabel,
  catalogStatusTone,
} from "@/lib/catalog-admin";

type Provider = {
  id: string;
  name: string;
  slug: string;
  kind?: string;
  adapter: string;
  health: string;
  status: string;
  account_count?: number;
};

export default function AdminProvidersPage() {
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="text-sm text-ink-secondary">
          提供商是 TokenHub 实际调用的上游，例如 OpenAI、Anthropic、火山方舟。列表展示健康状态、运行状态和账号池数量。凭据、已关联模型与同步操作在详情页。探测走上游沙箱、不会计费。提供商仅由平台接入，租户不能自带上游 Key。
        </p>
      </section>
      <AdminListPanel<Provider>
        path="/admin/providers"
        title="提供商"
        emptyTitle="还没有提供商"
        emptyDetail="由平台接入上游。不要在渠道里自建。"
        rowHref={(row) => providerHref(String(row.slug || row.id))}
        actions={
          <IfCan action="providers.write">
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              新建提供商
            </Button>
          </IfCan>
        }
        columns={[
          { accessorKey: "name", header: "名称" },
          {
            accessorKey: "slug",
            header: "标识",
            cell: ({ row }) => <span className="font-mono text-[13px]">{row.original.slug}</span>,
          },
          {
            accessorKey: "kind",
            header: "类型",
            cell: ({ row }) => providerKindLabel(row.original.kind),
          },
          {
            accessorKey: "adapter",
            header: "协议",
            cell: ({ row }) => adapterLabel(row.original.adapter),
          },
          {
            accessorKey: "health",
            header: "健康",
            cell: ({ row }) => <Badge tone={healthTone(row.original.health)}>{healthLabel(row.original.health)}</Badge>,
          },
          {
            accessorKey: "status",
            header: "状态",
            cell: ({ row }) => (
              <Badge tone={catalogStatusTone(row.original.status)}>{providerStatusLabel(row.original.status)}</Badge>
            ),
          },
          {
            accessorKey: "account_count",
            header: "账号池",
            cell: ({ row }) => row.original.account_count ?? 0,
          },
          {
            id: "probe",
            header: "探测",
            cell: ({ row }) => <ProbeCell id={String(row.original.id)} />,
          },
        ]}
      />
      <CreateProviderDialog open={createOpen} onOpenChange={setCreateOpen} />
    </AdminShell>
  );
}
