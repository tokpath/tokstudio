"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/empty-state";
import { ScrollTable } from "@/components/ui/scroll-table";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { CreateModelDialog } from "./create-dialog";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { type AdminModel, modelEditHref, vendorLabel } from "@/lib/catalog";
import { catalogStatusTone, formatProviderSlugs, modelStatusLabel, syncStateLabel } from "@/lib/catalog-admin";
import { CATALOG_HELP, CATALOG_LABEL } from "@/lib/catalog-copy";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type ListResponse = { items?: AdminModel[]; error?: { message?: string } };
type Tab = "catalog" | "review";

function ModelStatusCell({ status, syncState }: { status: string; syncState?: string }) {
  return (
    <div className="flex flex-col gap-1">
      <Badge tone={catalogStatusTone(status)}>{modelStatusLabel(status)}</Badge>
      <span className="text-xs text-ink-secondary">{syncStateLabel(syncState)}</span>
    </div>
  );
}

export default function AdminModelsPage() {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("catalog");
  const [syncState, setSyncState] = useState("draft");
  const [createOpen, setCreateOpen] = useState(false);
  const [message, setMessage] = useState("待审核由另一人通过后再发布。创建人不能审核或发布自己建的模型。");
  const path = syncState ? `/admin/models?sync_state=${encodeURIComponent(syncState)}` : "/admin/models";
  const query = useQuery({
    queryKey: [path],
    queryFn: async () => {
      const res = await fetch(`${apiBase}${path}`, { credentials: "include" });
      return (await res.json()) as ListResponse;
    },
    enabled: tab === "review",
  });
  const items = query.data?.items ?? [];

  async function review(id: string, action: "approve" | "reject") {
    const res = await fetch(`${apiBase}/admin/models/review`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ public_id: id, action }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已${action === "approve" ? "通过" : "拒绝"} ${body.item?.id}` : body.error?.message || "审核失败");
    await queryClient.invalidateQueries();
  }

  async function publish(id: string) {
    const res = await fetch(`${apiBase}/admin/models/publish`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ public_id: id }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已发布 ${body.item?.id} → ${body.item?.status}` : body.error?.message || "发布失败");
    await queryClient.invalidateQueries();
  }

  const createButton = (
    <IfCan action="models.write">
      <Button size="sm" onClick={() => setCreateOpen(true)}>
        创建模型
      </Button>
    </IfCan>
  );

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="max-w-3xl">
            <h2 className="text-lg font-semibold tracking-tight">模型</h2>
            <p className="mt-1 text-sm text-ink-secondary">{CATALOG_HELP.models}</p>
          </div>
          {createButton}
        </div>
        <ol className="mt-4 grid gap-3 sm:grid-cols-3">
          <li className="rounded-control border border-hairline bg-canvas p-4">
            <p className="text-xs font-medium uppercase tracking-wider text-ink-mute">1 公开模型</p>
            <p className="mt-1 text-sm leading-relaxed text-ink">给客户的名字和公开模型标识，例如 alibaba/happyhorse-1.0。</p>
          </li>
          <li className="rounded-control border border-hairline bg-canvas p-4">
            <p className="text-xs font-medium uppercase tracking-wider text-ink-mute">2 提供商</p>
            <p className="mt-1 text-sm leading-relaxed text-ink">选定进货渠道，并填写该渠道内部的上游模型标识。</p>
          </li>
          <li className="rounded-control border border-hairline bg-canvas p-4">
            <p className="text-xs font-medium uppercase tracking-wider text-ink-mute">3 路由组</p>
            <p className="mt-1 text-sm leading-relaxed text-ink">多家提供商时，在路由组里排提供商池和选路策略。</p>
          </li>
        </ol>
        <div className="mt-4 flex flex-wrap gap-2" role="tablist" aria-label="模型页签">
          <Button
            size="sm"
            role="tab"
            aria-selected={tab === "catalog"}
            variant={tab === "catalog" ? "default" : "outline"}
            onClick={() => setTab("catalog")}
          >
            目录
          </Button>
          <Button
            size="sm"
            role="tab"
            aria-selected={tab === "review"}
            variant={tab === "review" ? "default" : "outline"}
            onClick={() => setTab("review")}
          >
            审核
          </Button>
        </div>
      </section>

      {tab === "catalog" ? (
        <AdminListPanel<AdminModel>
          path="/admin/models"
          title="已上架与全部模型"
          emptyTitle="还没有公开模型"
          emptyDetail="点「创建模型」一次填好名字和提供商，或从提供商详情同步后再审核发布。"
          rowHref={(row) => modelEditHref(row.id)}
          columns={[
            {
              accessorKey: "id",
              header: CATALOG_LABEL.publicModelId,
              cell: ({ row }) => <span className="font-mono text-[13px]">{row.original.id}</span>,
            },
            { accessorKey: "display_name", header: CATALOG_LABEL.displayName },
            {
              accessorKey: "vendor",
              header: CATALOG_LABEL.vendor,
              cell: ({ row }) => vendorLabel(row.original.vendor),
            },
            {
              accessorKey: "status",
              header: "状态",
              cell: ({ row }) => (
                <Badge tone={catalogStatusTone(row.original.status)}>{modelStatusLabel(row.original.status)}</Badge>
              ),
            },
            {
              id: "providers",
              header: CATALOG_LABEL.providerPool,
              cell: ({ row }) => (
                <span className="font-mono text-[12px] text-ink-secondary">{formatProviderSlugs(row.original.providers)}</span>
              ),
            },
          ]}
        />
      ) : (
        <section className="rounded-card border border-hairline bg-canvas-raised p-6">
          <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
            <div>
              <AdminH2 k="modelReview" className="text-lg font-semibold tracking-tight" />
              <p className="mt-1 text-sm text-ink-secondary">通过后才能出现在客户目录。创建人和审核人必须是不同账号。</p>
            </div>
          </div>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Button size="sm" variant={syncState === "draft" ? "default" : "outline"} onClick={() => setSyncState("draft")}>
              待审核
            </Button>
            <Button size="sm" variant={syncState === "reviewed" ? "default" : "outline"} onClick={() => setSyncState("reviewed")}>
              已通过待发布
            </Button>
            <Button size="sm" variant={syncState === "rejected" ? "default" : "outline"} onClick={() => setSyncState("rejected")}>
              已拒绝
            </Button>
            <Button size="sm" variant={syncState === "" ? "default" : "outline"} onClick={() => setSyncState("")}>
              全部模型
            </Button>
          </div>
          {query.data?.error ? <p className="mb-3 text-sm text-ink-secondary">{query.data.error.message}</p> : null}
          <ScrollTable
            columns={[
              {
                id: "id",
                header: CATALOG_LABEL.publicModelId,
                cell: (item) => <span className="font-mono text-[13px]">{item.id}</span>,
              },
              { id: "display_name", header: CATALOG_LABEL.displayName, cell: (item) => item.display_name },
              { id: "vendor", header: CATALOG_LABEL.vendor, cell: (item) => vendorLabel(item.vendor) },
              {
                id: "providers",
                header: CATALOG_LABEL.providerPool,
                cell: (item) => (
                  <span className="font-mono text-[12px] text-ink-secondary">{formatProviderSlugs(item.providers)}</span>
                ),
              },
              {
                id: "status",
                header: "状态",
                cell: (item) => <ModelStatusCell status={item.status} syncState={item.sync_state} />,
              },
              {
                id: "actions",
                header: "操作",
                cell: (item) => (
                  <div className="flex flex-wrap gap-2">
                    <IfCan action="models.write">
                      {item.sync_state === "draft" || !item.sync_state ? (
                        <>
                          <ConfirmButton
                            size="sm"
                            title="确认通过模型"
                            description={`将通过 ${item.id}。创建人不能审核自己建的模型。`}
                            onConfirm={() => review(item.id, "approve")}
                          >
                            通过
                          </ConfirmButton>
                          <ConfirmButton
                            size="sm"
                            variant="outline"
                            title="确认拒绝模型"
                            description={`将拒绝 ${item.id}。`}
                            onConfirm={() => review(item.id, "reject")}
                          >
                            拒绝
                          </ConfirmButton>
                        </>
                      ) : null}
                      {item.sync_state === "reviewed" ? (
                        <ConfirmButton
                          size="sm"
                          title="确认发布模型"
                          description={`将发布 ${item.id} 到客户目录。创建人不能发布自己建的模型。`}
                          onConfirm={() => publish(item.id)}
                        >
                          发布
                        </ConfirmButton>
                      ) : null}
                      {item.sync_state === "rejected" ? (
                        <ConfirmButton size="sm" title="确认重新通过" description={`将重新通过 ${item.id}。`} onConfirm={() => review(item.id, "approve")}>
                          通过
                        </ConfirmButton>
                      ) : null}
                    </IfCan>
                    <Link className="inline-flex min-h-10 items-center text-sm text-brand-emphasis underline-offset-4 hover:underline" href={modelEditHref(item.id)}>
                      去改价和能力
                    </Link>
                  </div>
                ),
              },
            ]}
            rows={items}
            getRowId={(item) => item.id}
            empty={<EmptyState title="暂无待审模型" detail="点「创建模型」或从提供商同步后会出现在这里。" />}
          />
          <p className="mt-3 text-sm text-ink-secondary">{message}</p>
        </section>
      )}

      <CreateModelDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => setTab("review")}
      />
    </AdminShell>
  );
}
