"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { apiBase } from "@/lib/api";
import { dailyChartOption, requestChartOption } from "@/lib/charts";
import { dashboardHero, dashboardSummaryParams } from "@/lib/dashboard";
import { Download, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTranslations } from "next-intl";
import { AdminH2 } from "@/components/admin-h2";
import { MetricCard } from "@/components/feature-card";
import { DASHBOARD_HERO_ICONS } from "@/lib/page-icons";

type DashboardBody = {
  dashboard?: {
    totals?: Record<string, number>;
    alerts?: { kind?: string }[];
    dimensions?: Record<string, { key: string; requests?: number; revenue_minor?: number; success_rate?: number }[]>;
  };
  error?: { message?: string };
};

type SeriesBody = {
  items?: { day: string; requests?: number; revenue_minor?: number; success_rate?: number }[];
  error?: { message?: string };
};

export default function AdminDashboard() {
  const t = useTranslations("admin");
  const tu = useTranslations("adminUi");
  const td = useTranslations("dashboard");
  const [message, setMessage] = useState(td("lead"));
  const [dimension, setDimension] = useState("model");
  const chartRef = useRef<HTMLDivElement>(null);
  const seriesRef = useRef<HTMLDivElement>(null);
  const query = useQuery({
    queryKey: ["dashboard"],
    queryFn: async () => {
      const res = await fetch(`${apiBase}/admin/ops/dashboard`, { credentials: "include" });
      return (await res.json()) as DashboardBody;
    },
  });
  const seriesQuery = useQuery({
    queryKey: ["metrics-series"],
    queryFn: async () => {
      const res = await fetch(`${apiBase}/admin/metrics/series?days=7`, { credentials: "include" });
      return (await res.json()) as SeriesBody;
    },
  });

  async function refresh() {
    const [result, series] = await Promise.all([query.refetch(), seriesQuery.refetch()]);
    const body = result.data;
    if (!body?.dashboard) {
      setMessage(body?.error?.message || series.data?.error?.message || td("needAdmin"));
      return;
    }
    setMessage(td("summary", dashboardSummaryParams(body.dashboard)));
  }

  async function exportDaily() {
    const res = await fetch(`${apiBase}/admin/metrics/daily?format=csv&days=7`, { credentials: "include" });
    if (!res.ok) {
      setMessage(td("exportFail"));
      return;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "metrics-daily.csv";
    link.click();
    URL.revokeObjectURL(url);
    setMessage(td("exported"));
  }

  useEffect(() => {
    const items = query.data?.dashboard?.dimensions?.[dimension] ?? [];
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
  }, [query.data, dimension]);

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

  const hero = dashboardHero(query.data?.dashboard || {});

  return (
    <div className="flex flex-col gap-5">
      <section className="grid gap-3 md:grid-cols-2 lg:grid-cols-4" aria-label={tu("adminOverview")}>
        {hero.map((card) => (
          <MetricCard
            key={card.key}
            icon={DASHBOARD_HERO_ICONS[card.key] ?? DASHBOARD_HERO_ICONS.heroPending}
            label={t(card.key)}
            value={card.v}
            hint={t(card.hintKey)}
          />
        ))}
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="opsBoard" className="mb-2 text-xl font-medium tracking-tight" />
        <p className="mb-4 text-sm text-ink-secondary">{td("opsLead")}</p>
        <div className="flex flex-wrap gap-3">
          <label className="flex items-center gap-2 text-sm">
            <span className="text-ink-mute">{td("dimLabel")}</span>
            <select
              className="h-9 rounded-md border border-hairline bg-canvas px-3 text-sm"
              aria-label={td("dimLabel")}
              value={dimension}
              onChange={(e) => setDimension(e.target.value)}
            >
              <option value="model">{td("dimModel")}</option>
              <option value="api_key">{td("dimApiKey")}</option>
              <option value="channel">{td("dimChannel")}</option>
              <option value="user">{td("dimUser")}</option>
              <option value="provider">{td("dimProvider")}</option>
            </select>
          </label>
          <Button type="button" variant="outline" onClick={() => void refresh()}>
            <RefreshCw />
            {td("refreshMetrics")}
          </Button>
          <Button type="button" variant="outline" onClick={() => void exportDaily()}>
            <Download />
            {td("exportDaily")}
          </Button>
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <div className="rounded-card border border-hairline bg-canvas p-3">
            <p className="mb-2 text-xs uppercase tracking-[0.16em] text-ink-mute">
              {{
                model: td("dimModel"),
                api_key: td("dimApiKey"),
                channel: td("dimChannel"),
                user: td("dimUser"),
                provider: td("dimProvider"),
              }[dimension] || td("dimLabel")}
            </p>
            <div ref={chartRef} className="h-72 w-full" data-testid="ops-echarts" />
          </div>
          <div className="rounded-card border border-hairline bg-canvas p-3">
            <p className="mb-2 text-xs uppercase tracking-[0.16em] text-ink-mute">{td("last7d")}</p>
            <div ref={seriesRef} className="h-72 w-full" data-testid="ops-daily-chart" />
          </div>
        </div>
      </section>
    </div>
  );
}
