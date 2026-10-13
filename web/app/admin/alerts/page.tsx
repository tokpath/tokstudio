"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import { NotificationDeliveryLog } from "@/components/notifications/admin-settings";
import { useViewer } from "@/components/rbac/viewer-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Alert = { id: string; kind: string; severity: string; status: string; message: string };
type ListResponse = { items?: Alert[]; next_cursor?: string; error?: { message?: string } };

export default function AdminAlertsPage() {
  const [message, setMessage] = useState("按当前成功率、待对账和熔断状态生成告警。");
  const [error,setError]=useState("");
  const viewer = useViewer();
  const tc = useTranslations("common");
  const [hydrated, setHydrated] = useState(false);
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const [input, setInput] = useState("");
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  useEffect(() => {
    const restore = () => {
      const params = new URLSearchParams(window.location.search);
      const value = params.get("status") || "";
      setStatus(["open", "resolved", "closed"].includes(value) ? value : "");
      setSearch(params.get("q") || ""); setInput(params.get("q") || "");
      setCursor(params.get("cursor") || ""); setPrevious([]); setHydrated(true);
    };
    restore(); window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);
  function updateLocation(nextStatus: string, nextSearch: string, nextCursor: string) {
    const url = new URL(window.location.href);
    for (const [key, value] of [["status", nextStatus], ["q", nextSearch], ["cursor", nextCursor]]) {
      if (value) url.searchParams.set(key, value); else url.searchParams.delete(key);
    }
    window.history.replaceState(null, "", url.toString());
  }
  function filter(nextStatus: string, nextSearch: string) {
    setStatus(nextStatus); setSearch(nextSearch); setCursor(""); setPrevious([]);
    updateLocation(nextStatus, nextSearch, "");
  }
  const params = new URLSearchParams({ limit: "50" });
  if (status) params.set("status", status);
  if (search) params.set("q", search);
  if (cursor) params.set("cursor", cursor);
  const exportParams = new URLSearchParams({ format: "csv" });
  if (status) exportParams.set("status", status);
  if (search) exportParams.set("q", search);
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: [viewer.userId, "/admin/ops/alerts", status, search, cursor],
    queryFn: () => apiClient<ListResponse>("GET", `/admin/ops/alerts?${params}`),
    enabled: hydrated,
    retry: false,
  });
  const items = query.isError ? [] : query.data?.items ?? [];

  async function evaluate(): Promise<boolean> {
    setError("");
    try {
    const res = await fetch(`${apiBase}/admin/ops/alerts/evaluate`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
    });
    const body = await res.json();
    setMessage(res.ok ? "评估完成，已刷新告警列表。" : body.error?.message || "评估失败");
    if(!res.ok)setError(body.error?.message||"评估失败，请重试。");
    const __ok = res.ok;
    if (res.ok) { setCursor(""); setPrevious([]); updateLocation(status, search, ""); }
    await queryClient.invalidateQueries({ queryKey: [viewer.userId, "/admin/ops/alerts"] });
    return __ok;
    } catch {
      setError(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 level={1} k="alerts" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">按系统设置中的阈值检查当前运行情况，生成告警并保留审计记录。</p>
        <IfCan action="alerts.write">
        <ConfirmButton size="sm" title="确认评估告警" description="将按当前阈值重新评估运行情况并生成告警，不会自动修改上游配置或资金记录。" error={error} onConfirm={evaluate}>
          评估告警
        </ConfirmButton>
        </IfCan>
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2 text-sm">状态<select aria-label="告警状态" value={status} className="h-10 rounded-control border border-hairline bg-canvas px-3" onChange={event => filter(event.target.value, search)}><option value="">全部</option><option value="open">待处理</option><option value="resolved">已恢复</option><option value="closed">已关闭</option></select></label>
          <form className="flex min-w-0 items-center gap-2" onSubmit={event => { event.preventDefault(); filter(status, input.trim()); }}><Input className="w-44 sm:w-56" aria-label={tc("filter")} placeholder={tc("filter")} value={input} onChange={event => setInput(event.target.value)} /><Button size="sm" variant="outline" type="submit">{tc("search")}</Button></form>
          <Button asChild size="sm" variant="outline"><a href={`${apiBase}/admin/ops/alerts?${exportParams}`}>导出 CSV</a></Button>
        </div>
        {query.data?.error ? <p className="mt-3 text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        {query.isPending && <p role="status">正在读取告警…</p>}
        {query.isError && <div role="alert" className="mt-3"><p>读取告警失败，请重试。</p><Button size="sm" variant="outline" onClick={() => void query.refetch()}>{tc("listRetry")}</Button></div>}
        {!query.isPending && !query.isError && !query.data?.error && items.length===0 && <p className="mt-3">当前条件下暂无告警记录。</p>}
        <Link href="/admin/runbooks" className="mt-3 block text-brand-emphasis">查看应急处置手册</Link>
        <div className="mt-4 overflow-x-auto"><table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="px-2 py-2">类型</th>
              <th className="px-2 py-2">级别</th>
              <th className="px-2 py-2">状态</th>
              <th className="px-2 py-2">说明</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-hairline/80">
                <td className="px-2 py-2 text-ink">{({backup_drill_missing:"缺少备份恢复演练",low_success_rate:"成功率偏低",pending_reconciliation:"待对账",circuit_open:"上游熔断"} as Record<string,string>)[item.kind]||item.kind}</td>
                <td className="px-2 py-2 text-ink-secondary">{({critical:"严重",high:"高",medium:"中",low:"低"} as Record<string,string>)[item.severity]||item.severity}</td>
                <td className="px-2 py-2 text-ink-secondary">{({open:"待处理",resolved:"已恢复",closed:"已关闭"} as Record<string,string>)[item.status]||item.status}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.message}</td>
              </tr>
            ))}
          </tbody>
        </table></div>
        <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
          {cursor && <Button size="sm" variant="outline" disabled={query.isFetching} onClick={() => { const next = previous.at(-1) || ""; setPrevious(items => items.slice(0, -1)); setCursor(next); updateLocation(status, search, next); }}>{tc(previous.length ? "previousPage" : "firstPage")}</Button>}
          <span>{query.isFetching ? tc("listLoading") : query.isError || !query.data ? "—" : tc("pageCount", { count: items.length })}</span>
          {!query.isError && query.data?.next_cursor && <Button size="sm" variant="outline" disabled={query.isFetching} onClick={() => { const next = query.data?.next_cursor || ""; setPrevious(items => [...items, cursor]); setCursor(next); updateLocation(status, search, next); }}>{tc("nextPage")}</Button>}
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      <NotificationDeliveryLog />
    </AdminShell>
  );
}
