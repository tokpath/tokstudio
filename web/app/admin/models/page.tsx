"use client";

import { useForm } from "react-hook-form";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiClient } from "@/lib/client";

type Model = { id: string; vendor: string; display_name: string; status: string; sync_state?: string };

export default function AdminModelsPage() {
  const form = useForm({ defaultValues: { provider_id: "", public_id: "" } });
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
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="provider id 同步" {...form.register("provider_id")} />
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="public model id 审核并发布" {...form.register("public_id")} />
        <button className="rounded border border-slate-600 px-3 py-2" type="submit">
          同步 / 审核发布
        </button>
      </form>
    </AdminShell>
  );
}
