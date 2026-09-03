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
  formatCredentialRef,
  formatMappedModels,
  healthLabel,
  healthTone,
  providerHref,
  providerKindLabel,
  providerStatusLabel,
  catalogStatusTone,
  type MappedPublicModel,
} from "@/lib/catalog-admin";

type Provider = {
  id: string;
  name: string;
  slug: string;
  kind?: string;
  adapter: string;
  health: string;
  status: string;
  credential_ref?: string;
  models?: MappedPublicModel[];
};

export default function AdminProvidersPage() {
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="text-sm text-ink-secondary">
          提供商是 TokenHub 真正去调用的上游，例如 OpenAI、Anthropic、火山方舟。客户看到的是公开模型名（如 openai/gpt-5.6），一家提供商可以同时挂多个公开模型。
          点一行进入编辑页，在那里改状态、轮换上游 Key、管理账号池。列表每行可探测；探测走上游沙箱、不会计费，也不要二次确认。提供商只由平台接入，租户不能自带上游 Key。
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
            header: "适配器",
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
            accessorKey: "credential_ref",
            header: "凭据",
            cell: ({ row }) => formatCredentialRef(row.original.credential_ref),
          },
          {
            id: "models",
            header: "已挂模型",
            cell: ({ row }) => (
              <span className="font-mono text-[13px] text-ink-secondary">{formatMappedModels(row.original.models)}</span>
            ),
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
