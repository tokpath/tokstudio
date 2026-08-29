"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiClient } from "@/lib/client";

const schema = z.object({
  name: z.string().min(1),
  slug: z.string().min(1),
  adapter: z.string().min(1),
});

type Provider = { id: string; name: string; slug: string; adapter: string; health: string; status: string };

export default function AdminProvidersPage() {
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { name: "", slug: "", adapter: "test" } });
  async function onSubmit(values: z.infer<typeof schema>) {
    await apiClient("POST", "/admin/providers", {
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify(values),
    });
  }
  return (
    <AdminShell>
      <AdminListPanel<Provider>
        path="/admin/providers"
        title="提供商"
        columns={[
          { accessorKey: "slug", header: "Slug" },
          { accessorKey: "adapter", header: "Adapter" },
          { accessorKey: "health", header: "Health" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
      <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-slate-800 p-4" onSubmit={form.handleSubmit(onSubmit)}>
        <p className="text-sm text-slate-400">创建 Provider 需要二次确认头，密钥不会回显。</p>
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="name" {...form.register("name")} />
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="slug" {...form.register("slug")} />
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="adapter" {...form.register("adapter")} />
        <button className="rounded border border-slate-600 px-3 py-2" type="submit">
          创建
        </button>
      </form>
    </AdminShell>
  );
}
