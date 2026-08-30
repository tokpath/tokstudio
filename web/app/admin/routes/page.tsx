"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type Route = { id: string; public_model_id: string; strategy: string; status: string };

export default function AdminRoutesPage() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("创建和改策略都要二次确认。不要改 rg_echo，那是文本网关默认路由。");

  async function createRoute(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const publicID = String(data.get("public_model_id") || "").trim();
    const strategy = String(data.get("strategy") || "priority").trim() || "priority";
    const status = String(data.get("status") || "active").trim() || "active";
    const providerID = String(data.get("provider_id") || "").trim();
    const body: Record<string, unknown> = { public_model_id: publicID, strategy, status };
    if (providerID) {
      body.candidates = [{ provider_id: providerID, priority: 1, weight: 1 }];
    }
    const res = await fetch(`${apiBase}/admin/routes`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify(body),
    });
    const json = await res.json();
    if (!res.ok) {
      setMessage(json.error?.message || "创建失败");
      return;
    }
    form.reset();
    setMessage(`已创建 ${json.item?.id} → ${json.item?.strategy} / ${json.item?.status}`);
    await queryClient.invalidateQueries();
  }

  async function patchRoute(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const routeID = String(data.get("route_id") || "").trim();
    const strategy = String(data.get("strategy") || "").trim();
    const status = String(data.get("status") || "").trim();
    const res = await fetch(`${apiBase}/admin/routes/${routeID}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ strategy, status }),
    });
    const json = await res.json();
    if (!res.ok) {
      setMessage(json.error?.message || "保存失败");
      return;
    }
    setMessage(`已保存 ${json.item?.id} → ${json.item?.strategy} / ${json.item?.status}`);
    await queryClient.invalidateQueries();
  }

  return (
    <AdminShell>
      <AdminListPanel<Route>
        path="/admin/routes"
        title="路由组"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "public_model_id", header: "Model" },
          { accessorKey: "strategy", header: "Strategy" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
      <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-slate-800 bg-slate-900/70 p-4" onSubmit={createRoute}>
        <h2 className="text-xl font-medium">创建路由</h2>
        <p className="text-sm text-slate-400">给已有公开模型建一个路由组。策略可选 priority / weight / price / health。</p>
        <Input name="public_model_id" aria-label="创建用 public model id" placeholder="创建用 public model id" />
        <Input name="strategy" aria-label="创建用策略" placeholder="创建用策略 priority" defaultValue="priority" />
        <Input name="status" aria-label="创建用状态" placeholder="创建用状态 active" defaultValue="active" />
        <Input name="provider_id" aria-label="创建用 provider id" placeholder="创建用 provider id（可选候选）" />
        <Button size="sm" type="submit">
          创建路由
        </Button>
      </form>
      <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-slate-800 bg-slate-900/70 p-4" onSubmit={patchRoute}>
        <h2 className="text-xl font-medium">改路由策略</h2>
        <p className="text-sm text-slate-400">只改策略或状态。不要对 rg_echo 乱改，改完会改变 echo 网关的选路。</p>
        <Input name="route_id" aria-label="改策略用路由 id" placeholder="改策略用路由 id" />
        <Input name="strategy" aria-label="改策略用策略" placeholder="改策略用策略 health" />
        <Input name="status" aria-label="改策略用状态" placeholder="改策略用状态 active" />
        <Button size="sm" type="submit">
          保存策略
        </Button>
      </form>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </AdminShell>
  );
}
