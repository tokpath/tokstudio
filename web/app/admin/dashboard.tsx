"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { apiBase } from "@/lib/api";
import { requestChartOption } from "@/lib/charts";
import { formatDashboard } from "@/lib/dashboard";

type DashboardBody = {
  dashboard?: {
    totals?: Record<string, number>;
    alerts?: { kind?: string }[];
    dimensions?: { model?: { key: string; requests?: number; revenue_minor?: number; success_rate?: number }[] };
  };
  error?: { message?: string };
};

export default function AdminDashboard() {
  const [message, setMessage] = useState("按 Provider / 模型 / 渠道 / 用户 / API Key 看成功率、延迟、收入和毛利。");
  const chartRef = useRef<HTMLDivElement>(null);
  const query = useQuery({
    queryKey: ["dashboard"],
    enabled: false,
    queryFn: async () => {
      const res = await fetch(`${apiBase}/admin/ops/dashboard`, { credentials: "include" });
      return (await res.json()) as DashboardBody;
    },
  });

  async function refresh() {
    const result = await query.refetch();
    const body = result.data;
    if (!body?.dashboard) {
      setMessage(body?.error?.message || "未登录平台管理员");
      return;
    }
    setMessage(formatDashboard(body.dashboard));
  }

  useEffect(() => {
    const items = query.data?.dashboard?.dimensions?.model ?? [];
    if (!chartRef.current || items.length === 0) {
      return;
    }
    let disposed = false;
    import("echarts").then((echarts) => {
      if (disposed || !chartRef.current) {
        return;
      }
      const chart = echarts.init(chartRef.current);
      chart.setOption(requestChartOption(items));
      return () => chart.dispose();
    });
    return () => {
      disposed = true;
    };
  }, [query.data]);

  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <h2 className="mb-3 text-xl font-medium">运营看板</h2>
      <p className="mb-3 text-sm text-slate-400">告警和 runbook 在 /admin/ops/alerts 与 /admin/ops/runbooks。图表使用 Apache ECharts。</p>
      <button className="rounded border border-slate-600 px-4 py-2" onClick={refresh}>
        刷新指标
      </button>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
      <div ref={chartRef} className="mt-4 h-72 w-full" data-testid="ops-echarts" />
    </section>
  );
}
