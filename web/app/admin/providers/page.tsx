"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

const schema = z.object({
  name: z.string().min(1),
  slug: z.string().min(1),
  adapter: z.string().min(1),
});

type Provider = { id: string; name: string; slug: string; adapter: string; health: string; status: string; credential_ref?: string };

function ProbeCell({ id }: { id: string }) {
  const queryClient = useQueryClient();
  const [result, setResult] = useState("");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        size="sm"
        variant="outline"
        onClick={async () => {
          const res = await fetch(`${apiBase}/admin/providers/${id}/health-check`, {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json" },
            body: "{}",
          });
          const body = await res.json();
          setResult(res.ok ? String(body.health ?? "ok") : body.error?.message || "探测失败");
          await queryClient.invalidateQueries();
        }}
      >
        探测
      </Button>
      {result ? <span className="text-xs text-slate-400">{result}</span> : null}
    </div>
  );
}

function RotateCredentialForm() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("轮换需要二次确认头。响应和列表只回 credential_ref，不会回显明文。");

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const providerID = String(data.get("provider_id") || "").trim();
    const secret = String(data.get("secret") || "");
    const res = await fetch(`${apiBase}/admin/providers/${providerID}/credentials`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ secret }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "轮换失败");
      return;
    }
    form.reset();
    setMessage(`已轮换，credential_ref=${body.credential_ref || ""}`);
    await queryClient.invalidateQueries();
  }

  return (
    <form className="mt-4 rounded-2xl border border-slate-800 bg-slate-900/70 p-4" onSubmit={onSubmit}>
      <h2 className="mb-3 text-xl font-medium">凭据轮换</h2>
      <p className="mb-3 text-sm text-slate-400">旧密文立即标记 rotated。不要对生产主 Provider 随便试，先建一次性提供商。</p>
      <div className="mb-3 flex flex-wrap gap-2">
        <Input className="w-72" name="provider_id" aria-label="轮换 provider id" placeholder="轮换用 provider id" />
        <Input className="w-72" type="password" name="secret" aria-label="上游凭据" placeholder="轮换用密文" autoComplete="new-password" />
        <Button size="sm" type="submit">
          轮换凭据
        </Button>
      </div>
      <p className="text-sm text-slate-300">{message}</p>
    </form>
  );
}

function AccountPoolForm() {
  const form = useForm({
    defaultValues: { provider_id: "", secret: "", label: "primary" },
  });
  async function onSubmit(values: { provider_id: string; secret: string; label: string }) {
    await apiClient("POST", `/admin/providers/${values.provider_id}/accounts`, {
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ secret: values.secret, label: values.label, kind: "api_key" }),
    });
  }
  return (
    <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-slate-800 p-4" onSubmit={form.handleSubmit(onSubmit)}>
      <p className="text-sm text-slate-400">上游账号池只保存密文，列表只显示指纹。冷却或失效账号不会被路由选中。</p>
      <input className="rounded bg-slate-900 px-3 py-2" placeholder="provider id" {...form.register("provider_id")} />
      <input className="rounded bg-slate-900 px-3 py-2" placeholder="label" {...form.register("label")} />
      <input className="rounded bg-slate-900 px-3 py-2" placeholder="upstream secret" type="password" {...form.register("secret")} />
      <button className="rounded border border-slate-600 px-3 py-2" type="submit">
        添加账号
      </button>
    </form>
  );
}

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
      <p className="text-sm text-slate-400">列表每行可探测。探测走上游沙箱、不会计费，也不要二次确认。</p>
      <AdminListPanel<Provider>
        path="/admin/providers"
        title="提供商"
        columns={[
          { accessorKey: "slug", header: "Slug" },
          { accessorKey: "adapter", header: "Adapter" },
          { accessorKey: "health", header: "Health" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "credential_ref", header: "Cred Ref" },
          {
            id: "probe",
            header: "探测",
            cell: ({ row }) => <ProbeCell id={String(row.original.id)} />,
          },
        ]}
      />
      <RotateCredentialForm />
      <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-slate-800 p-4" onSubmit={form.handleSubmit(onSubmit)}>
        <p className="text-sm text-slate-400">创建 Provider 需要二次确认头，密钥不会回显。</p>
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="name" {...form.register("name")} />
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="slug" {...form.register("slug")} />
        <input className="rounded bg-slate-900 px-3 py-2" placeholder="adapter" {...form.register("adapter")} />
        <button className="rounded border border-slate-600 px-3 py-2" type="submit">
          创建
        </button>
      </form>
      <AccountPoolForm />
    </AdminShell>
  );
}
