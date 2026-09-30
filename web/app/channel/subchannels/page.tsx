"use client";

import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction } from "@/lib/rbac";
import { useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import type { PlanChannel } from "@/lib/plan-channels";
import { OEMPage } from "../management-panels";

async function loadChannels(): Promise<PlanChannel[]> {
  const items: PlanChannel[] = [];
  let cursor = "";
  do {
    const page = await apiClient<{ items?: PlanChannel[]; next_cursor?: string }>("GET", `/admin/channels?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);
    items.push(...(page.items ?? []));
    if (!page.next_cursor || page.next_cursor === cursor) break;
    cursor = page.next_cursor;
  } while (true);
  return items;
}

export default function SubchannelsPage() {
  const queryClient = useQueryClient();
  const viewer = useViewer();
  const [code, setCode] = useState("");
  const [message, setMessage] = useState("");
  const [creating, setCreating] = useState(false);
  const me = useQuery({ queryKey: ["/channel/me"], queryFn: () => apiClient<{ channel_org_id: string; channel_type: string }>("GET", "/channel/me") });
  const channels = useQuery({ queryKey: ["/channel/subchannels"], queryFn: loadChannels });
  const children = channels.data?.filter((item) => item.type === "B" && item.parent_id === me.data?.channel_org_id) ?? [];

  async function create() {
    if (!code.trim() || creating) return;
    setCreating(true);
    setMessage("");
    try {
      const response = await fetch(`${apiBase}/admin/channels`, {
        method: "POST", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ code: code.trim(), type: "B", status: "active" }),
      });
      const body = await response.json();
      if (!response.ok) {
        setMessage(body.error?.message || "创建失败");
        return;
      }
      setCode("");
      setMessage(`已创建 ${body.item?.code || "渠道"}，请继续授权模型和管理员。`);
      await queryClient.invalidateQueries({ queryKey: ["/channel/subchannels"] });
    } catch {
      setMessage(confirmNetworkUnavailable);
    } finally {
      setCreating(false);
    }
  }

  return <OEMPage page="channels">
    <div className="flex flex-wrap items-center gap-4 text-sm">{viewer.roles.some((role) => ["channel_admin", "oem_ops", "oem_audit"].includes(role)) ? <Link href="/channel/models" className="text-brand-emphasis underline">本品牌模型授权</Link> : null}<span className="text-ink-secondary">下属渠道共用本品牌套餐与价格，模型按渠道授权。</span></div>
    <section className="rounded-card border border-hairline bg-canvas-raised p-5">
      <h2 className="font-semibold">创建渠道</h2>
      <div className="mt-3 flex max-w-lg gap-2"><Input aria-label="新渠道名称" placeholder="渠道名称" value={code} onChange={(event) => setCode(event.target.value)} /><Button disabled={!canChannelAction("operations", viewer) || !code.trim() || creating} onClick={() => void create()}>{creating ? "创建中…" : "创建"}</Button></div>
      {message ? <p role="status" className="mt-2 text-sm text-ink-secondary">{message}</p> : null}
    </section>
    <section className="rounded-card border border-hairline bg-canvas-raised p-5">
      <div className="flex items-center justify-between"><h2 className="font-semibold">渠道列表</h2><Button size="sm" variant="outline" onClick={() => void channels.refetch()}>刷新</Button></div>
      {channels.isPending || me.isPending ? <p role="status" className="mt-4">正在读取渠道…</p> : null}
      {channels.isError || me.isError ? <p role="alert" className="mt-4">读取渠道失败，请重试。</p> : null}
      {!channels.isPending && !me.isPending && !children.length ? <p className="mt-4 text-sm text-ink-secondary">暂无下属渠道。</p> : null}
      <ul className="mt-3 divide-y divide-hairline">{children.map((item) => <li key={item.id} className="flex items-center justify-between gap-4 py-3"><span><span className="font-medium">{item.code}</span><span className="ml-3 text-sm text-ink-secondary">{item.status === "active" ? "运行中" : "已停用"}</span></span><Link className="text-sm text-brand-emphasis hover:underline" href={`/channel/subchannels/${encodeURIComponent(item.id)}`}>管理 →</Link></li>)}</ul>
    </section>
  </OEMPage>;
}
