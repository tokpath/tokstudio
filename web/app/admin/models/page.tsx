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
import { Form } from "@/components/ui/form";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { type AdminModel, formatSellPrice, modelEditHref } from "@/lib/catalog";
import { AdminH2 } from "@/components/admin-h2";

type ListResponse = { items?: AdminModel[]; error?: { message?: string } };

const createSchema = z.object({
  public_id: z.string().trim().min(1, "请填写 public id"),
  vendor: z.string().trim().min(1, "请填写厂商"),
  display_name: z.string().trim().min(1, "请填写显示名"),
});

const syncSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写 provider id"),
});

const attachSchema = z.object({
  public_id: z.string().trim().min(1, "请填写 public id"),
  provider_id: z.string().trim().min(1, "请填写 provider id"),
  upstream_model_id: z.string().trim().min(1, "请填写 upstream model id"),
});

const deprecateSchema = z.object({
  public_id: z.string().trim().min(1, "请填写 public id"),
});

export default function AdminModelsPage() {
  const queryClient = useQueryClient();
  const [syncState, setSyncState] = useState("draft");
  const [message, setMessage] = useState("同步只进入 draft。通过、拒绝、发布要分开做，创建人不能审核或发布自己建的模型。");
  const [createMessage, setCreateMessage] = useState("手工创建永远是 draft。客户目录要先换人审核再发布。不要改 tokenhub/echo-1。");
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { public_id: "", vendor: "tokenhub", display_name: "" },
  });
  const syncForm = useForm<z.infer<typeof syncSchema>>({
    resolver: zodResolver(syncSchema),
    defaultValues: { provider_id: "" },
  });
  const attachForm = useForm<z.infer<typeof attachSchema>>({
    resolver: zodResolver(attachSchema),
    defaultValues: { public_id: "", provider_id: "", upstream_model_id: "" },
  });
  const deprecateForm = useForm<z.infer<typeof deprecateSchema>>({
    resolver: zodResolver(deprecateSchema),
    defaultValues: { public_id: "" },
  });
  const path = syncState ? `/admin/models?sync_state=${encodeURIComponent(syncState)}` : "/admin/models";
  const query = useQuery({
    queryKey: [path],
    queryFn: async () => {
      const res = await fetch(`${apiBase}${path}`, { credentials: "include" });
      return (await res.json()) as ListResponse;
    },
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
      <p className="text-sm text-ink-secondary">
        提供商和公开模型只在平台目录维护。租户不能自己添加提供商或模型，只能由平台把已有目录授权给渠道白名单。列表和编辑都走后端
        catalog，不是 mock。点「编辑」改属性、定价和上架。不要改 tokenhub/echo-1。
      </p>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="modelReview" className="mb-3 text-xl font-medium" />
        <p className="mb-3 text-sm text-ink-secondary">
          同步和手工创建先进待审核。通过后才能发布到客户目录。创建人和审核人必须是不同账号。
        </p>
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
        {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="px-2 py-2">Public ID</th>
              <th className="px-2 py-2">Vendor</th>
              <th className="px-2 py-2">Name</th>
              <th className="px-2 py-2">Status</th>
              <th className="px-2 py-2">Sync</th>
              <th className="px-2 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-hairline/80">
                <td className="px-2 py-2 text-ink">{item.id}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.vendor}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.display_name}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.status}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.sync_state || "-"}</td>
                <td className="px-2 py-2">
                  <div className="flex flex-wrap gap-2">
                    {item.sync_state === "draft" || !item.sync_state ? (
                      <>
                        <ConfirmButton size="sm" title="确认通过模型" description={`将通过 ${item.id}，并写入审计。创建人不能审核自己建的模型。`} onConfirm={() => review(item.id, "approve")}>
                          通过
                        </ConfirmButton>
                        <ConfirmButton size="sm" variant="outline" title="确认拒绝模型" description={`将拒绝 ${item.id}，并写入审计。`} onConfirm={() => review(item.id, "reject")}>
                          拒绝
                        </ConfirmButton>
                      </>
                    ) : null}
                    {item.sync_state === "reviewed" ? (
                      <ConfirmButton size="sm" title="确认发布模型" description={`将发布 ${item.id} 到客户目录。创建人不能发布自己建的模型。`} onConfirm={() => publish(item.id)}>
                        发布
                      </ConfirmButton>
                    ) : null}
                    {item.sync_state === "rejected" ? (
                      <ConfirmButton size="sm" title="确认重新通过" description={`将重新通过 ${item.id}。`} onConfirm={() => review(item.id, "approve")}>
                        通过
                      </ConfirmButton>
                    ) : null}
                    <Link className="text-brand-emphasis underline-offset-4 hover:underline" href={modelEditHref(item.id)}>
                      编辑
                    </Link>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      <AdminListPanel<AdminModel>
        path="/admin/models"
        title="模型"
        columns={[
          { accessorKey: "id", header: "Public ID" },
          { accessorKey: "vendor", header: "Vendor" },
          { accessorKey: "display_name", header: "Name" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "sync_state", header: "Sync" },
          {
            id: "sell_price",
            header: "Sell",
            cell: ({ row }) => formatSellPrice(row.original.sell_price),
          },
          {
            id: "edit",
            header: "操作",
            cell: ({ row }) => (
              <Link className="text-brand-emphasis underline-offset-4 hover:underline" href={modelEditHref(row.original.id)}>
                编辑
              </Link>
            ),
          },
        ]}
      />
      <Form {...createForm}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="createModel" className="text-xl font-medium" />
          <p className="text-sm text-ink-secondary">缺确认会 409。永远创建为 draft，不会立刻出现在客户目录。</p>
          <TextField control={createForm.control} name="public_id" label="创建用 public id" placeholder="创建用 public id tokenhub/ops-ui" />
          <TextField control={createForm.control} name="vendor" label="创建用厂商" placeholder="创建用厂商 tokenhub" />
          <TextField control={createForm.control} name="display_name" label="创建用显示名" />
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
              await queryClient.invalidateQueries();
            })}
          >
            创建模型
          </ConfirmButton>
          <p className="text-sm text-ink-secondary">{createMessage}</p>
        </form>
      </Form>
      <Form {...syncForm}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
          <p className="text-sm text-ink-secondary">同步只写入 draft。请换另一个运营账号去「模型审核」里通过或拒绝。</p>
          <TextField control={syncForm.control} name="provider_id" label="provider id 同步" placeholder="provider id 同步" />
          <ConfirmButton
            size="sm"
            variant="outline"
            title="确认同步"
            description="同步进入 draft，不会自动审核或发布。"
            validate={() => syncForm.trigger()}
            onConfirm={syncForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/providers/${values.provider_id}/sync`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: "{}",
              });
              const body = await res.json();
              const ids = (body.item?.items || []).map((item: AdminModel) => item.id).join(", ");
              setMessage(res.ok ? `已同步 draft：${ids || values.provider_id}` : body.error?.message || "同步失败");
              await queryClient.invalidateQueries();
            })}
          >
            同步
          </ConfirmButton>
        </form>
      </Form>
      <Form {...attachForm}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="mountProvider" className="text-xl font-medium" />
          <p className="text-sm text-ink-secondary">把已有公开模型挂到 Provider，upstream 名称可以和公开 ID 不同。</p>
          <TextField control={attachForm.control} name="public_id" label="挂载 public id" placeholder="public_id" />
          <TextField control={attachForm.control} name="provider_id" label="挂载 provider id" placeholder="provider_id" />
          <TextField control={attachForm.control} name="upstream_model_id" label="upstream model id" placeholder="upstream_model_id" />
          <ConfirmButton
            size="sm"
            title="确认挂载 Provider"
            description="upstream 名称可以和公开 ID 不同。"
            validate={() => attachForm.trigger()}
            onConfirm={attachForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/models/attach`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify(values),
              });
              const body = await res.json();
              setMessage(res.ok ? `已挂载 ${values.public_id} → ${values.provider_id}` : body.error?.message || "挂载失败");
            })}
          >
            挂载
          </ConfirmButton>
        </form>
      </Form>
      <Form {...deprecateForm}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="deprecateModel" className="text-xl font-medium" />
          <p className="text-sm text-ink-secondary">只改状态，不删除历史映射和价格版本。</p>
          <TextField control={deprecateForm.control} name="public_id" label="弃用 public id" placeholder="public_id" />
          <ConfirmButton
            size="sm"
            title="确认弃用模型"
            description="只改状态，不删除历史映射和价格版本。"
            validate={() => deprecateForm.trigger()}
            onConfirm={deprecateForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/models/deprecate`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ public_id: values.public_id }),
              });
              const body = await res.json();
              setMessage(res.ok ? `已弃用 ${body.item?.id || values.public_id} → ${body.item?.status}` : body.error?.message || "弃用失败");
            })}
          >
            弃用模型
          </ConfirmButton>
        </form>
      </Form>
    </AdminShell>
  );
}
