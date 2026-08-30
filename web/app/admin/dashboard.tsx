"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { apiBase } from "@/lib/api";
import { dailyChartOption, requestChartOption } from "@/lib/charts";
import { formatDashboard } from "@/lib/dashboard";

type DashboardBody = {
  dashboard?: {
    totals?: Record<string, number>;
    alerts?: { kind?: string }[];
    dimensions?: { model?: { key: string; requests?: number; revenue_minor?: number; success_rate?: number }[] };
  };
  error?: { message?: string };
};

type SeriesBody = {
  items?: { day: string; requests?: number; revenue_minor?: number; success_rate?: number }[];
  error?: { message?: string };
};

export default function AdminDashboard() {
  const [message, setMessage] = useState("按 Provider / 模型 / 渠道 / 用户 / API Key 看成功率、延迟、收入和毛利。");
  const chartRef = useRef<HTMLDivElement>(null);
  const seriesRef = useRef<HTMLDivElement>(null);
  const query = useQuery({
    queryKey: ["dashboard"],
    enabled: false,
    queryFn: async () => {
      const res = await fetch(`${apiBase}/admin/ops/dashboard`, { credentials: "include" });
      return (await res.json()) as DashboardBody;
    },
  });
  const seriesQuery = useQuery({
    queryKey: ["metrics-series"],
    enabled: false,
    queryFn: async () => {
      const res = await fetch(`${apiBase}/admin/metrics/series?days=7`, { credentials: "include" });
      return (await res.json()) as SeriesBody;
    },
  });

  async function refresh() {
    const [result, series] = await Promise.all([query.refetch(), seriesQuery.refetch()]);
    const body = result.data;
    if (!body?.dashboard) {
      setMessage(body?.error?.message || series.data?.error?.message || "未登录平台管理员");
      return;
    }
    setMessage(formatDashboard(body.dashboard));
  }

  async function exportDaily() {
    const res = await fetch(`${apiBase}/admin/metrics/daily?format=csv&days=7`, { credentials: "include" });
    if (!res.ok) {
      setMessage("导出失败，请先登录平台管理员");
      return;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "metrics-daily.csv";
    link.click();
    URL.revokeObjectURL(url);
    setMessage("已导出近 7 日报汇总 CSV");
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

  useEffect(() => {
    const items = seriesQuery.data?.items ?? [];
    if (!seriesRef.current || items.length === 0) {
      return;
    }
    let disposed = false;
    import("echarts").then((echarts) => {
      if (disposed || !seriesRef.current) {
        return;
      }
      const chart = echarts.init(seriesRef.current);
      chart.setOption(dailyChartOption(items));
      return () => chart.dispose();
    });
    return () => {
      disposed = true;
    };
  }, [seriesQuery.data]);

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6 ">
      <h2 className="mb-2 text-xl font-medium tracking-tight">运营看板</h2>
      <p className="mb-4 text-sm text-ink-secondary">告警和应急手册在 /admin/alerts 与 /admin/runbooks。时间序列来自网关与账务接口，不直连业务表。</p>
      <div className="flex flex-wrap gap-3">
        <button className="rounded-lg border border-hairline px-4 py-2 text-sm hover:bg-canvas-raised" onClick={refresh}>
          刷新指标
        </button>
        <button className="rounded-lg border border-hairline px-4 py-2 text-sm hover:bg-canvas-raised" onClick={exportDaily}>
          导出日报 CSV
        </button>
      </div>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <div className="rounded-xl border border-hairline bg-canvas p-3">
          <p className="mb-2 text-xs uppercase tracking-[0.16em] text-ink-mute">模型请求</p>
          <div ref={chartRef} className="h-72 w-full" data-testid="ops-echarts" />
        </div>
        <div className="rounded-xl border border-hairline bg-canvas p-3">
          <p className="mb-2 text-xs uppercase tracking-[0.16em] text-ink-mute">近 7 日</p>
          <div ref={seriesRef} className="h-72 w-full" data-testid="ops-daily-chart" />
        </div>
      </div>
    </section>
  );
}
