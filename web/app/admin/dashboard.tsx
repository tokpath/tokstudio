"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useTheme } from "next-themes";
import { apiBase } from "@/lib/api";
import { chartPalette, dailyChartOption, requestChartOption } from "@/lib/charts";
import { dashboardHero, dashboardSummaryParams } from "@/lib/dashboard";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTranslations } from "next-intl";
import { AdminH2 } from "@/components/admin-h2";
import { MetricCard } from "@/components/feature-card";
import { EChart } from "@/components/echart";
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
  const tChart = useTranslations("charts");
  const { resolvedTheme } = useTheme();
  const [message, setMessage] = useState(td("lead"));
  const [dimension, setDimension] = useState("model");
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

  const palette = useMemo(() => chartPalette(resolvedTheme === "dark"), [resolvedTheme]);
  const labels = useMemo(() => ({ requests: tChart("requests"), revenue: tChart("spend") }), [tChart]);
  const dimItems = query.data?.dashboard?.dimensions?.[dimension] ?? [];
  const dayItems = seriesQuery.data?.items ?? [];
  const dimOption = useMemo(
    () => (dimItems.length ? requestChartOption(dimItems, labels, palette) : null),
    [dimItems, labels, palette],
  );
  const dayOption = useMemo(
    () => (dayItems.length ? dailyChartOption(dayItems, labels, palette) : null),
    [dayItems, labels, palette],
  );

  const hero = dashboardHero(query.data?.dashboard || {});
  const dimTitle =
    {
      model: td("dimModel"),
      api_key: td("dimApiKey"),
      channel: td("dimChannel"),
      user: td("dimUser"),
      provider: td("dimProvider"),
    }[dimension] || td("dimLabel");

  return (
    <div className="flex flex-col gap-5">
      <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-4" aria-label={tu("adminOverview")}>
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
        <AdminH2 k="opsBoard" className="mb-4 text-lg font-semibold tracking-tight" />
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
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <div className="rounded-card border border-hairline bg-canvas p-3">
            <p className="mb-2 text-xs uppercase tracking-[0.16em] text-ink-mute">{dimTitle}</p>
            <EChart
              option={dimOption}
              emptyTitle={tChart("empty")}
              emptyDetail={tChart("emptyDetail")}
              testId="ops-echarts"
            />
          </div>
          <div className="rounded-card border border-hairline bg-canvas p-3">
            <p className="mb-2 text-xs uppercase tracking-[0.16em] text-ink-mute">{td("last7d")}</p>
            <EChart
              option={dayOption}
              emptyTitle={tChart("empty")}
              emptyDetail={tChart("emptyDetail")}
              testId="ops-daily-chart"
            />
          </div>
        </div>
      </section>
    </div>
  );
}
