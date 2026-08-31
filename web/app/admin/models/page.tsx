"use client";

import { useState } from "react";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { type AdminModel, formatSellPrice, modelEditHref } from "@/lib/catalog";
import { AdminH2 } from "@/components/admin-h2";

const createSchema = z.object({
  public_id: z.string().trim().min(1, "请填写 public id"),
  vendor: z.string().trim().min(1, "请填写厂商"),
  display_name: z.string().trim().min(1, "请填写显示名"),
  status: z.string().trim().min(1, "请填写状态"),
});

const syncSchema = z.object({
  provider_id: z.string().trim(),
  public_id: z.string().trim(),
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
  const [message, setMessage] = useState("同步进入 draft，审核发布后客户才看得到。弃用不删历史映射和价格。");
  const [createMessage, setCreateMessage] = useState("手工创建默认 draft。客户目录要先审核再发布。不要改 tokenhub/echo-1。");
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { public_id: "", vendor: "tokenhub", display_name: "", status: "draft" },
  });
  const syncForm = useForm<z.infer<typeof syncSchema>>({
    resolver: zodResolver(syncSchema),
    defaultValues: { provider_id: "", public_id: "" },
  });
  const attachForm = useForm<z.infer<typeof attachSchema>>({
    resolver: zodResolver(attachSchema),
    defaultValues: { public_id: "", provider_id: "", upstream_model_id: "" },
  });
  const deprecateForm = useForm<z.infer<typeof deprecateSchema>>({
    resolver: zodResolver(deprecateSchema),
    defaultValues: { public_id: "" },
  });

  return (
    <AdminShell>
      <p className="text-sm text-ink-secondary">列表和编辑都走后端 catalog，不是 mock。点「编辑」改属性、定价和上架。不要改 tokenhub/echo-1。</p>
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
          <p className="text-sm text-ink-secondary">缺确认会 409。默认 draft，不会立刻出现在客户目录。</p>
          <TextField control={createForm.control} name="public_id" label="创建用 public id" placeholder="创建用 public id tokenhub/ops-ui" />
          <TextField control={createForm.control} name="vendor" label="创建用厂商" placeholder="创建用厂商 tokenhub" />
          <TextField control={createForm.control} name="display_name" label="创建用显示名" />
          <TextField control={createForm.control} name="status" label="创建用状态" placeholder="创建用状态 draft" />
          <ConfirmButton
            size="sm"
            title="确认创建模型"
            description="默认 draft，不会立刻出现在客户目录。不要改 tokenhub/echo-1。"
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
              setCreateMessage(`已创建 ${body.item?.id} → ${body.item?.status}`);
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
          <p className="text-sm text-ink-secondary">同步结果先进入 draft，审核通过后再发布到客户目录。</p>
          <TextField control={syncForm.control} name="provider_id" label="provider id 同步" placeholder="provider id 同步" />
          <TextField control={syncForm.control} name="public_id" label="public model id 审核并发布" placeholder="public model id 审核并发布" />
          <ConfirmButton
            size="sm"
            variant="outline"
            title="确认同步 / 审核发布"
            description="同步进入 draft，再审核并发布到客户目录。"
            validate={() => syncForm.trigger()}
            onConfirm={syncForm.handleSubmit(async (values) => {
              if (values.provider_id) {
                await apiClient("POST", `/admin/providers/${values.provider_id}/sync`, {
                  headers: confirmHeaders,
                  body: "{}",
                });
              }
              if (values.public_id) {
                await apiClient("POST", "/admin/models/review", {
                  headers: confirmHeaders,
                  body: JSON.stringify({ public_id: values.public_id, action: "approve" }),
                });
                await apiClient("POST", "/admin/models/publish", {
                  headers: confirmHeaders,
                  body: JSON.stringify({ public_id: values.public_id }),
                });
              }
              setMessage(`已处理 ${values.provider_id || values.public_id || "空表单"}`);
            })}
          >
            同步 / 审核发布
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
      <p className="text-sm text-ink-secondary">{message}</p>
    </AdminShell>
  );
}
