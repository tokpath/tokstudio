"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import { useTranslations } from "next-intl";
import { ConfirmButton } from "@/components/confirm-button";
import { Input } from "@/components/ui/input";
import { EChart } from "@/components/echart";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { chartPalette, dailyChartOption, requestChartOption } from "@/lib/charts";
import { type UsageEvent, groupUsageByDay, groupUsageByModel } from "@/lib/usage";
import { statementListPath } from "@/lib/reconciliation";
import { IfCan } from "@/components/rbac/if-can";

type Usage = {
  id: string;
  request_id: string;
  user_id?: string;
  api_key_id?: string;
  public_model_id?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  customer_amount_minor?: number;
  state: string;
};

export default function AdminUsagePage() {
  const tChart = useTranslations("charts");
  const { resolvedTheme } = useTheme();
  const [requestID, setRequestID] = useState("");
  const [prompt, setPrompt] = useState("8");
  const [completion, setCompletion] = useState("4");
  const [apiKeyID, setApiKeyID] = useState("");
  const [userID, setUserID] = useState("");
  const [modelID, setModelID] = useState("");
  const [channelID, setChannelID] = useState("");
  const [state, setState] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [message, setMessage] = useState("待对账必须按真实 usage 回放，禁止按估算扣款。");

  async function replay(): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/admin/usage/replay`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        request_id: requestID,
        usage: { prompt_tokens: Number(prompt), completion_tokens: Number(completion) },
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已回放 ${body.item?.usage_event_id || requestID} → ${body.item?.state}` : body.error?.message || "回放失败");
    const __ok = res.ok;
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  const listPath = statementListPath({
    apiKeyId: apiKeyID,
    userId: userID,
    modelId: modelID,
    channelId: channelID,
    state,
    from,
    to,
  });
  const chartQuery = useQuery({
    queryKey: ["admin-usage-chart", listPath],
    queryFn: () => apiClient<{ items?: UsageEvent[] }>("GET", `${listPath}${listPath.includes("?") ? "&" : "?"}limit=100`),
  });
  const palette = useMemo(() => chartPalette(resolvedTheme === "dark"), [resolvedTheme]);
  const labels = useMemo(() => ({ requests: tChart("requests"), revenue: tChart("spend") }), [tChart]);
  const events = chartQuery.data?.items ?? [];
  const trendOption = useMemo(() => {
    const points = groupUsageByDay(events);
    return points.length ? dailyChartOption(points, labels, palette) : null;
  }, [events, labels, palette]);
  const modelOption = useMemo(() => {
    const points = groupUsageByModel(events);
    return points.length ? requestChartOption(points, labels, palette) : null;
  }, [events, labels, palette]);

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="usage" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">对 pending_reconciliation 按 request_id 补真实 Token。重复回放幂等，不会双扣。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-72" value={requestID} onChange={(e) => setRequestID(e.target.value)} aria-label="request_id" placeholder="request_id" />
          <Input className="w-24" value={prompt} onChange={(e) => setPrompt(e.target.value)} aria-label="prompt tokens" />
          <Input className="w-24" value={completion} onChange={(e) => setCompletion(e.target.value)} aria-label="completion tokens" />
          <IfCan action="usage.replay">
          <ConfirmButton size="sm" title="确认回放 usage" description="只对 pending_reconciliation 按真实 Token 回放，不会双扣。" onConfirm={replay}>
            回放 usage
          </ConfirmButton>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
        <p className="mt-2 text-sm text-ink-mute">
          <Link href="/admin/reconciliation" className="underline-offset-2 hover:underline">
            待对账队列
          </Link>
        </p>
        <div className="mt-5 grid gap-4 lg:grid-cols-2">
          <div className="rounded-card border border-hairline bg-canvas p-3">
            <h3 className="mb-2 text-sm font-medium">{tChart("trend")}</h3>
            <EChart
              option={trendOption}
              emptyTitle={tChart("empty")}
              emptyDetail={tChart("emptyDetail")}
              testId="admin-usage-trend-chart"
            />
          </div>
          <div className="rounded-card border border-hairline bg-canvas p-3">
            <h3 className="mb-2 text-sm font-medium">{tChart("byModel")}</h3>
            <EChart
              option={modelOption}
              emptyTitle={tChart("empty")}
              emptyDetail={tChart("emptyDetail")}
              testId="admin-usage-model-chart"
            />
          </div>
        </div>
      </section>
      <AdminListPanel<Usage>
        path={listPath}
        title="用量 / 账单"
        actions={
          <div className="flex flex-wrap gap-2">
            <Input
              className="w-44"
              value={apiKeyID}
              onChange={(e) => setApiKeyID(e.target.value)}
              aria-label="按 API Key 筛选"
              placeholder="api_key_id"
            />
            <Input
              className="w-40"
              value={userID}
              onChange={(e) => setUserID(e.target.value)}
              aria-label="按用户筛选"
              placeholder="user_id"
            />
            <Input
              className="w-44"
              value={modelID}
              onChange={(e) => setModelID(e.target.value)}
              aria-label="按模型筛选"
              placeholder="public_model_id"
            />
            <Input
              className="w-40"
              value={channelID}
              onChange={(e) => setChannelID(e.target.value)}
              aria-label="按渠道筛选"
              placeholder="channel_id"
            />
            <Input
              className="w-36"
              value={state}
              onChange={(e) => setState(e.target.value)}
              aria-label="按状态筛选"
              placeholder="state"
            />
            <Input type="date" className="w-36" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="起始日期" />
            <Input type="date" className="w-36" value={to} onChange={(e) => setTo(e.target.value)} aria-label="结束日期" />
          </div>
        }
        columns={[
          { accessorKey: "occurred_at", header: "时间" },
          { accessorKey: "api_key_id", header: "API Key" },
          { accessorKey: "public_model_id", header: "模型" },
          { accessorKey: "prompt_tokens", header: "输入" },
          { accessorKey: "completion_tokens", header: "输出" },
          { accessorKey: "customer_amount_minor", header: "金额" },
          { accessorKey: "state", header: "State" },
          { accessorKey: "request_id", header: "Request" },
        ]}
      />
    </AdminShell>
  );
}
