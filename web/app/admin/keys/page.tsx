"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type Key = { id: string; user_id: string; name: string; prefix: string; rpm_limit: number; status: string };

export default function AdminKeysPage() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("禁用要二次确认。列表只有 prefix，没有完整密钥。不要禁正在跑 e2e 的主 Key。");

  async function onDisable(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const id = String(data.get("key_id") || "").trim();
    const res = await fetch(`${apiBase}/admin/api-keys/${id}/disable`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: "{}",
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "禁用失败");
      return;
    }
    form.reset();
    setMessage(`已禁用 ${body.item?.id} → ${body.item?.status} / ${body.item?.prefix || ""}`);
    await queryClient.invalidateQueries();
  }

  return (
    <AdminShell>
      <form className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6" onSubmit={onDisable}>
        <h2 className="mb-3 text-xl font-medium">禁用 API Key</h2>
        <p className="mb-3 text-sm text-slate-400">平台管理员和技术值班可以禁任意用户的 Key。禁用后网关立刻 403，不会回显密文。</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <Input name="key_id" aria-label="禁用用 API Key ID" placeholder="禁用用 API Key ID" />
        </div>
        <Button size="sm" type="submit">
          禁用 Key
        </Button>
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </form>
      <AdminListPanel<Key>
        path="/admin/api-keys"
        title="API Key"
        columns={[
          { accessorKey: "prefix", header: "Prefix" },
          { accessorKey: "name", header: "Name" },
          { accessorKey: "user_id", header: "User" },
          { accessorKey: "rpm_limit", header: "RPM" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
