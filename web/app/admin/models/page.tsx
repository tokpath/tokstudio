"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Form } from "@/components/ui/form";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { EmptyState } from "@/components/empty-state";
import { ScrollTable } from "@/components/ui/scroll-table";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { type AdminModel, modelEditHref } from "@/lib/catalog";
import { catalogStatusTone, formatProviderSlugs, statusWord, syncStateLabel } from "@/lib/catalog-admin";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type ListResponse = { items?: AdminModel[]; error?: { message?: string } };
type Tab = "catalog" | "review";

const createSchema = z.object({
  public_id: z.string().trim().min(1, "请填写公开 ID"),
  vendor: z.string().trim().min(1, "请填写厂商"),
  display_name: z.string().trim().min(1, "请填写显示名"),
});

function ModelStatusCell({ status, syncState }: { status: string; syncState?: string }) {
  return (
    <div className="flex flex-col gap-1">
      <Badge tone={catalogStatusTone(status)}>{statusWord(status)}</Badge>
      <span className="text-xs text-ink-secondary">{syncStateLabel(syncState)}</span>
    </div>
  );
}

export default function AdminModelsPage() {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("catalog");
  const [syncState, setSyncState] = useState("draft");
  const [createOpen, setCreateOpen] = useState(false);
  const [message, setMessage] = useState("同步只进入 draft。通过、拒绝、发布要分开做，创建人不能审核或发布自己建的模型。");
  const [createMessage, setCreateMessage] = useState("手工创建永远是 draft。客户目录要先换人审核再发布。不要改 tokenhub/echo-1。");
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { public_id: "", vendor: "tokenhub", display_name: "" },
  });
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

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="text-sm text-ink-secondary">
          公开模型是客户看到的货架名。目录 Tab 只看公开 ID、显示名、状态和途径；挂载、弃用和改价在详情。审核 Tab
          处理 draft / 已通过 / 已拒绝，并可手工创建。同步上游在提供商详情。不要改 tokenhub/echo-1。
        </p>
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
          title="模型目录"
          emptyTitle="还没有公开模型"
          emptyDetail="从提供商同步或手工创建，审核后再发布到客户目录。"
          rowHref={(row) => modelEditHref(row.id)}
          columns={[
            {
              accessorKey: "id",
              header: "公开 ID",
              cell: ({ row }) => <span className="font-mono text-[13px]">{row.original.id}</span>,
            },
            { accessorKey: "display_name", header: "显示名" },
            {
              accessorKey: "status",
              header: "状态",
              cell: ({ row }) => <Badge tone={catalogStatusTone(row.original.status)}>{statusWord(row.original.status)}</Badge>,
            },
            {
              id: "providers",
              header: "途径",
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
              <p className="mt-1 text-sm text-ink-secondary">同步和手工创建先进待审核。通过后才能发布到客户目录。创建人和审核人必须是不同账号。</p>
            </div>
            <IfCan action="models.write">
              <Button size="sm" onClick={() => setCreateOpen(true)}>
                创建模型
              </Button>
            </IfCan>
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
                header: "公开 ID",
                cell: (item) => <span className="font-mono text-[13px]">{item.id}</span>,
              },
              { id: "display_name", header: "显示名", cell: (item) => item.display_name },
              {
                id: "providers",
                header: "途径",
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
                            description={`将通过 ${item.id}，并写入审计。创建人不能审核自己建的模型。`}
                            onConfirm={() => review(item.id, "approve")}
                          >
                            通过
                          </ConfirmButton>
                          <ConfirmButton
                            size="sm"
                            variant="outline"
                            title="确认拒绝模型"
                            description={`将拒绝 ${item.id}，并写入审计。`}
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
                    <Link className="text-brand-emphasis underline-offset-4 hover:underline" href={modelEditHref(item.id)}>
                      编辑
                    </Link>
                  </div>
                ),
              },
            ]}
            rows={items}
            getRowId={(item) => item.id}
            empty={<EmptyState title="暂无待审模型" detail="从提供商同步或点「创建模型」后会出现在这里。" />}
          />
          <p className="mt-3 text-sm text-ink-secondary">{message}</p>
        </section>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建模型</DialogTitle>
            <DialogDescription>
              缺确认会 409。永远创建为 draft，不会立刻出现在客户目录。公开 ID 写成 厂商/模型，左边是原厂，不是进货渠道。
            </DialogDescription>
          </DialogHeader>
          <Form {...createForm}>
            <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
              <TextField control={createForm.control} name="public_id" label="公开 ID" placeholder="例如 tokenhub/ops-ui" />
              <TextField control={createForm.control} name="vendor" label="厂商" placeholder="模型原厂，如 openai / tokenhub" />
              <TextField control={createForm.control} name="display_name" label="显示名" />
              <ConfirmButton
                size="sm"
                title="确认创建模型"
                description="永远创建为 draft，不会立刻出现在客户目录。不要改 tokenhub/echo-1。"
                validate={() => createForm.trigger()}
                onConfirm={createForm.handleSubmit(async (values) => {
                  const res = await fetch(`${apiBase}/admin/models`, {
                    method: "POST",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: JSON.stringify(values),
                  });
                  const body = await res.json();
                  if (!res.ok) {
                    setCreateMessage(body.error?.message || "创建失败");
                    return;
                  }
                  createForm.reset();
                  setCreateMessage(`已创建 ${body.item?.id} → ${body.item?.status} / ${body.item?.sync_state}`);
                  setCreateOpen(false);
                  setTab("review");
                  await queryClient.invalidateQueries();
                })}
              >
                创建模型
              </ConfirmButton>
              <p className="text-sm text-ink-secondary">{createMessage}</p>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}
