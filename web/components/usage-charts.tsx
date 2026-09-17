"use client";

import { useMemo } from "react";
import { useTheme } from "next-themes";
import { useTranslations } from "next-intl";
import { EChart } from "@/components/echart";
import { chartPalette, dailyChartOption, requestChartOption, type MetricPoint } from "@/lib/charts";
import { type UsageEvent, groupUsageByDay, groupUsageByModel } from "@/lib/usage";

/** 用量两张图：按日趋势 + 按 Key/模型分布。总览与用量页复用；表格对账留在用量页。 */
export function UsageCharts({
  events,
  breakdown,
  breakdownTitle,
  testIdPrefix = "usage",
}: {
  events: UsageEvent[];
  breakdown?: MetricPoint[];
  breakdownTitle: string;
  /** 总览用 overview，用量页默认 usage，避免同一 DOM 里 testId 撞车。 */
  testIdPrefix?: string;
}) {
  const t = useTranslations("charts");
  const { resolvedTheme } = useTheme();
  const dark = resolvedTheme === "dark";
  const palette = useMemo(() => chartPalette(dark), [dark]);
  const labels = useMemo(() => ({ requests: t("requests"), revenue: t("spend") }), [t]);
  const trend = useMemo(() => groupUsageByDay(events), [events]);
  const bars = useMemo(() => breakdown ?? groupUsageByModel(events), [breakdown, events]);
  const trendOption = useMemo(
    () => (trend.some((row) => row.requests > 0 || row.revenue_minor > 0) ? dailyChartOption(trend, labels, palette) : null),
    [trend, labels, palette],
  );
  const barOption = useMemo(
    () => (bars.some((row) => (row.requests ?? 0) > 0 || (row.revenue_minor ?? 0) > 0) ? requestChartOption(bars, labels, palette) : null),
    [bars, labels, palette],
  );

  return (
    <div
      className="mb-5 grid gap-4 lg:grid-cols-2"
      data-testid={`${testIdPrefix}-charts-scope`}
      data-event-count={events.length}
    >
      <div className="rounded-card border border-hairline bg-canvas p-4">
        <h3 className="mb-2 text-sm font-medium">{t("trend")}</h3>
        <EChart
          option={trendOption}
          emptyTitle={t("empty")}
          emptyDetail={t("emptyDetail")}
          testId={`${testIdPrefix}-trend-chart`}
        />
      </div>
      <div className="rounded-card border border-hairline bg-canvas p-4">
        <h3 className="mb-2 text-sm font-medium">{breakdownTitle}</h3>
        <EChart
          option={barOption}
          emptyTitle={t("empty")}
          emptyDetail={t("emptyDetail")}
          testId={`${testIdPrefix}-breakdown-chart`}
        />
      </div>
    </div>
  );
}
