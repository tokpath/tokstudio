"use client";

import { useState } from "react";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { CreateChannelDialog, CreatePartnerDialog, OpenCreateButton } from "./create-dialogs";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  channelHref,
  channelTypeLabel,
  partnerHref,
  roleTypeLabel,
  type TenantListKind,
} from "@/lib/tenants";
import { canWrite } from "@/lib/rbac";
import { useViewer } from "@/components/rbac/viewer-context";
import { IfCan } from "@/components/rbac/if-can";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string; parent_id?: string };
type Role = { id: string; channel_org_id: string; type: string; parent_id?: string; level?: number; status: string };

const tabs: { kind: TenantListKind; label: string }[] = [
  { kind: "channel", label: "渠道" },
  { kind: "agent", label: "代理商" },
  { kind: "kol", label: "KOL" },
];

export default function AdminChannelsPage() {
  const [kind, setKind] = useState<TenantListKind>("channel");
  const [createChannel, setCreateChannel] = useState(false);
  const [createAgent, setCreateAgent] = useState(false);
  const [createKOL, setCreateKOL] = useState(false);
  const viewer = useViewer();
  const showPartners = canWrite("partners.view", viewer);

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="mb-4 text-lg font-semibold tracking-tight">渠道租户</h2>
        <p className="text-sm text-ink-secondary">
          渠道组织是多租户边界（A 官方 / B 批发 / C OEM）。代理商和 KOL 是租户内推广角色，字段不同，不是同类主体。
          所有租户的模型资源只能从平台目录出发，不能自己添加提供商和模型。新建渠道会复制平台已启用白名单。点进详情可编辑并保存渠道；B/C 租户详情可调整额度。
        </p>
      </section>
      <div className="flex flex-wrap gap-2" role="tablist" aria-label="分销主体类型">
        {(showPartners ? tabs : tabs.filter((tab) => tab.kind === "channel")).map((tab) => (
          <Button
            key={tab.kind}
            size="sm"
            role="tab"
            aria-selected={kind === tab.kind}
            variant={kind === tab.kind ? "default" : "outline"}
            onClick={() => setKind(tab.kind)}
          >
            {tab.label}
          </Button>
        ))}
      </div>
      {kind === "channel" ? (
        <AdminListPanel<Channel>
          path="/admin/channels"
          title="渠道租户"
          rowHref={(row) => channelHref(String(row.id))}
          emptyTitle="还没有渠道租户"
          emptyDetail="点新建渠道，从平台目录复制模型白名单。"
          actions={<IfCan action="channels.write"><OpenCreateButton label="新建渠道" onClick={() => setCreateChannel(true)} /></IfCan>}
          columns={[
            { accessorKey: "code", header: "Code" },
            {
              accessorKey: "type",
              header: "租户类型",
              cell: ({ row }) => channelTypeLabel(String(row.original.type)),
            },
            { accessorKey: "status", header: "状态" },
            { accessorKey: "brand_id", header: "品牌" },
            { accessorKey: "id", header: "ID" },
          ]}
        />
      ) : null}
      {kind === "agent" ? (
        <AdminListPanel<Role>
          path="/admin/acquisition-roles?type=agent"
          title="代理商"
          rowHref={(row) => partnerHref(String(row.id))}
          emptyTitle="还没有代理商"
          emptyDetail="代理商挂在某个渠道租户下，可发展 1 级 KOL。"
          actions={<IfCan action="partners.write"><OpenCreateButton label="新建代理商" onClick={() => setCreateAgent(true)} /></IfCan>}
          columns={[
            { accessorKey: "id", header: "ID" },
            { accessorKey: "channel_org_id", header: "所属租户" },
            {
              accessorKey: "type",
              header: "角色",
              cell: ({ row }) => roleTypeLabel(String(row.original.type)),
            },
            { accessorKey: "status", header: "状态" },
            { accessorKey: "level", header: "层级" },
          ]}
        />
      ) : null}
      {kind === "kol" ? (
        <AdminListPanel<Role>
          path="/admin/acquisition-roles?type=kol"
          title="KOL"
          rowHref={(row) => partnerHref(String(row.id))}
          emptyTitle="还没有 KOL"
          emptyDetail="1 级可发展 2 级；2 级不能再发展下级。"
          actions={<IfCan action="partners.write"><OpenCreateButton label="新建 KOL" onClick={() => setCreateKOL(true)} /></IfCan>}
          columns={[
            { accessorKey: "id", header: "ID" },
            {
              accessorKey: "type",
              header: "层级",
              cell: ({ row }) => <Badge tone="brand">{roleTypeLabel(String(row.original.type))}</Badge>,
            },
            { accessorKey: "channel_org_id", header: "所属租户" },
            { accessorKey: "parent_id", header: "上级" },
            { accessorKey: "status", header: "状态" },
            { accessorKey: "level", header: "Level" },
          ]}
        />
      ) : null}
      <CreateChannelDialog open={createChannel} onOpenChange={setCreateChannel} />
      <CreatePartnerDialog open={createAgent} onOpenChange={setCreateAgent} defaultType="agent" />
      <CreatePartnerDialog open={createKOL} onOpenChange={setCreateKOL} defaultType="kol_l1" />
    </AdminShell>
  );
}
