"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
import { formatUsdMinor } from "@/lib/money";
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
  const queryClient = useQueryClient();
  const [replayError, setReplayError] = useState("");
  const validTokens = (value: string) => /^\d+$/.test(value) && Number.isSafeInteger(Number(value));
  const tChart = useTranslations("charts");
  const { resolvedTheme } = useTheme();
  const [requestID, setRequestID] = useState("");
  const [prompt, setPrompt] = useState("");
  const [completion, setCompletion] = useState("");
  const [apiKeyID, setApiKeyID] = useState("");
  const [userID, setUserID] = useState("");
  const [modelID, setModelID] = useState("");
  const [channelID, setChannelID] = useState("");
  const [state, setState] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [message, setMessage] = useState("待对账必须按真实 usage 回放，禁止按估算扣款。");

  async function replay(): Promise<boolean> {
    setReplayError("");
    if (!requestID.trim() || !validTokens(prompt) || !validTokens(completion)) {
      setReplayError("请填写请求编号及真实的非负整数 Token 数量。"); return false;
    }
    try {
    const res = await fetch(`${apiBase}/admin/usage/replay`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        request_id: requestID.trim(),
        usage: { prompt_tokens: Number(prompt), completion_tokens: Number(completion) },
      }),
    });
    const body = await res.json();
    if (res.ok) {
      setMessage(`已完成请求 ${requestID.trim()} 的用量回放，请在账单中核对结果。`);
      void queryClient.invalidateQueries();
    } else { setReplayError(body.error?.message || "回放失败，请核对请求编号和上游用量后重试。"); }
    const __ok = res.ok;
    return __ok;
    } catch {
      setReplayError(confirmNetworkUnavailable);
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
        <p className="mb-3 text-sm text-ink-secondary">仅用于待对账请求：根据上游凭证补录真实输入和输出 Token，确认后将结算消费。重复回放不会重复扣款。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-72" value={requestID} onChange={(e) => setRequestID(e.target.value)} aria-label="消费请求编号" placeholder="消费请求编号" />
          <Input className="w-24" value={prompt} onChange={(e) => setPrompt(e.target.value)} aria-label="真实输入 Token" placeholder="输入 Token" inputMode="numeric" />
          <Input className="w-24" value={completion} onChange={(e) => setCompletion(e.target.value)} aria-label="真实输出 Token" placeholder="输出 Token" inputMode="numeric" />
          <IfCan action="usage.replay">
          <ConfirmButton size="sm" title="确认补录真实用量" description={`请求 ${requestID.trim()}；输入 ${prompt} Token，输出 ${completion} Token。将按真实用量结算消费，请先核对上游凭证。`} error={replayError} validate={() => { setReplayError(""); return true; }} disabled={!requestID.trim() || !validTokens(prompt) || !validTokens(completion)} onConfirm={replay}>
            补录真实用量
          </ConfirmButton>
          </IfCan>
        </div>
        <p role="status" className="text-sm text-ink-secondary">{message}</p>
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
        title="用量 / 账单（原消费金额保留，已退款记录不计入消费汇总）"
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
          { accessorKey: "customer_amount_minor", header: "原消费金额（USD）", cell: ({ row }) => formatUsdMinor(row.original.customer_amount_minor) },
          { accessorKey: "state", header: "状态", cell: ({ row }) => ({ confirmed: "已结算", voided: "已退款", pending_reconciliation: "待对账", reserved: "已预授权", failed: "失败" }[row.original.state] || row.original.state) },
          { accessorKey: "request_id", header: "消费请求编号" },
        ]}
      />
    </AdminShell>
  );
}
