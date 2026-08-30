"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

type Model = { id: string; vendor: string; display_name: string; status: string; sync_state?: string };

export default function AdminModelsPage() {
  const queryClient = useQueryClient();
  const form = useForm({ defaultValues: { provider_id: "", public_id: "" } });
  const [message, setMessage] = useState("同步进入 draft，审核发布后客户才看得到。弃用不删历史映射和价格。");
  const [createMessage, setCreateMessage] = useState("手工创建默认 draft。客户目录要先审核再发布。不要改 tokenhub/echo-1。");

  async function attach(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formEl = event.currentTarget;
    const data = new FormData(formEl);
    const res = await fetch(`${apiBase}/admin/models/attach`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({
        public_id: String(data.get("public_id") || "").trim(),
        provider_id: String(data.get("provider_id") || "").trim(),
        upstream_model_id: String(data.get("upstream_model_id") || "").trim(),
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已挂载 ${data.get("public_id")} → ${data.get("provider_id")}` : body.error?.message || "挂载失败");
  }

  async function deprecate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formEl = event.currentTarget;
    const data = new FormData(formEl);
    const publicID = String(data.get("public_id") || "").trim();
    const res = await fetch(`${apiBase}/admin/models/deprecate`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ public_id: publicID }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已弃用 ${body.item?.id || publicID} → ${body.item?.status}` : body.error?.message || "弃用失败");
  }

  return (
    <AdminShell>
      <AdminListPanel<Model>
        path="/admin/models"
        title="模型"
        columns={[
          { accessorKey: "id", header: "Public ID" },
          { accessorKey: "vendor", header: "Vendor" },
          { accessorKey: "display_name", header: "Name" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "sync_state", header: "Sync" },
        ]}
      />
      <form
        className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-4"
        onSubmit={async (event) => {
          event.preventDefault();
          const formEl = event.currentTarget;
          const data = new FormData(formEl);
          const publicID = String(data.get("public_id") || "").trim();
          const res = await fetch(`${apiBase}/admin/models`, {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
            body: JSON.stringify({
              public_id: publicID,
              vendor: String(data.get("vendor") || "").trim(),
              display_name: String(data.get("display_name") || "").trim(),
              status: String(data.get("status") || "draft").trim() || "draft",
            }),
          });
          const body = await res.json();
          if (!res.ok) {
            setCreateMessage(body.error?.message || "创建失败");
            return;
          }
          formEl.reset();
          setCreateMessage(`已创建 ${body.item?.id} → ${body.item?.status}`);
          await queryClient.invalidateQueries();
        }}
      >
        <h2 className="text-xl font-medium">创建模型</h2>
        <p className="text-sm text-slate-400">缺确认会 409。默认 draft，不会立刻出现在客户目录。</p>
        <Input name="public_id" aria-label="创建用 public id" placeholder="创建用 public id tokenhub/ops-ui" />
        <Input name="vendor" aria-label="创建用厂商" placeholder="创建用厂商 tokenhub" defaultValue="tokenhub" />
        <Input name="display_name" aria-label="创建用显示名" placeholder="创建用显示名" />
        <Input name="status" aria-label="创建用状态" placeholder="创建用状态 draft" defaultValue="draft" />
        <Button size="sm" type="submit">
          创建模型
        </Button>
        <p className="text-sm text-slate-300">{createMessage}</p>
      </form>
      <form
        className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-slate-800 p-4"
        onSubmit={form.handleSubmit(async (values) => {
          if (values.provider_id) {
            await apiClient("POST", `/admin/providers/${values.provider_id}/sync`, {
              headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
              body: "{}",
            });
          }
          if (values.public_id) {
            await apiClient("POST", "/admin/models/review", {
              headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
              body: JSON.stringify({ public_id: values.public_id, action: "approve" }),
            });
            await apiClient("POST", "/admin/models/publish", {
              headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
              body: JSON.stringify({ public_id: values.public_id }),
            });
          }
        })}
      >
        <p className="text-sm text-slate-400">同步结果先进入 draft，审核通过后再发布到客户目录。</p>
        <input className="rounded-lg border border-white/10 bg-black/30 px-3 py-2" placeholder="provider id 同步" {...form.register("provider_id")} />
        <input className="rounded-lg border border-white/10 bg-black/30 px-3 py-2" placeholder="public model id 审核并发布" {...form.register("public_id")} />
        <button className="rounded border border-slate-600 px-3 py-2" type="submit">
          同步 / 审核发布
        </button>
      </form>
      <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-4" onSubmit={attach}>
        <h2 className="text-xl font-medium">挂载 Provider</h2>
        <p className="text-sm text-slate-400">把已有公开模型挂到 Provider，upstream 名称可以和公开 ID 不同。</p>
        <Input name="public_id" aria-label="挂载 public id" placeholder="public_id" />
        <Input name="provider_id" aria-label="挂载 provider id" placeholder="provider_id" />
        <Input name="upstream_model_id" aria-label="upstream model id" placeholder="upstream_model_id" />
        <Button size="sm" type="submit">
          挂载
        </Button>
      </form>
      <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-4" onSubmit={deprecate}>
        <h2 className="text-xl font-medium">弃用模型</h2>
        <p className="text-sm text-slate-400">只改状态，不删除历史映射和价格版本。</p>
        <Input name="public_id" aria-label="弃用 public id" placeholder="public_id" />
        <Button size="sm" type="submit">
          弃用模型
        </Button>
      </form>
      <p className="text-sm text-slate-300">{message}</p>
    </AdminShell>
  );
}
