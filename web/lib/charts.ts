export type MetricPoint = { key: string; requests?: number; revenue_minor?: number; success_rate?: number };
export type DayPoint = { day: string; requests?: number; revenue_minor?: number; success_rate?: number };

export type ChartLabels = {
  requests?: string;
  revenue?: string;
};

export type ChartPalette = {
  brand: string;
  contrast: string;
  mute: string;
  hairline: string;
  raised: string;
  ink: string;
};

const TABULAR = "ui-monospace, 'JetBrains Mono', 'Geist Mono', monospace";

/** 纸/碳两套轴色，对应 DESIGN.md colors。 */
export function chartPalette(dark = false): ChartPalette {
  if (dark) {
    return {
      brand: "#2150D6",
      contrast: "#C4BDB2",
      mute: "#8A8378",
      hairline: "#2A2723",
      raised: "#1F1D1A",
      ink: "#EDEAE4",
    };
  }
  return {
    brand: "#2150D6",
    contrast: "#4A453C",
    mute: "#7A7368",
    hairline: "#D9D3C7",
    raised: "#FFFDF8",
    ink: "#141414",
  };
}

export function shortChartLabel(value: string, max = 18) {
  if (value.length <= max) {
    return value;
  }
  return `${value.slice(0, max - 1)}…`;
}

function axisLabel(palette: ChartPalette) {
  return { color: palette.mute, fontFamily: TABULAR, fontSize: 11 };
}

function tooltip(palette: ChartPalette) {
  return {
    trigger: "axis" as const,
    backgroundColor: palette.raised,
    borderColor: palette.hairline,
    borderWidth: 1,
    textStyle: { color: palette.ink, fontFamily: TABULAR, fontSize: 12 },
  };
}

function valueAxis(palette: ChartPalette, name = "") {
  return {
    type: "value" as const,
    name,
    nameTextStyle: axisLabel(palette),
    axisLabel: axisLabel(palette),
    axisLine: { show: false },
    splitLine: { lineStyle: { color: palette.hairline, width: 1 } },
  };
}

export function dashboardSeries(items: MetricPoint[] = []) {
  return {
    categories: items.map((item) => shortChartLabel(item.key)),
    requests: items.map((item) => item.requests ?? 0),
    revenue: items.map((item) => item.revenue_minor ?? 0),
    success: items.map((item) => Number(((item.success_rate ?? 0) * 100).toFixed(1))),
  };
}

export function requestChartOption(items: MetricPoint[], labels: ChartLabels = {}, palette: ChartPalette = chartPalette()) {
  const series = dashboardSeries(items);
  const requests = labels.requests || "requests";
  const revenue = labels.revenue || "revenue";
  return {
    color: [palette.brand, palette.contrast],
    tooltip: tooltip(palette),
    legend: { data: [requests, revenue], textStyle: axisLabel(palette), top: 0 },
    grid: { left: 48, right: 52, top: 36, bottom: 28, containLabel: true },
    xAxis: {
      type: "category",
      data: series.categories,
      axisLabel: axisLabel(palette),
      axisLine: { lineStyle: { color: palette.hairline } },
      axisTick: { show: false },
    },
    yAxis: [valueAxis(palette, requests), { ...valueAxis(palette, revenue), splitLine: { show: false } }],
    series: [
      {
        name: requests,
        type: "bar",
        data: series.requests,
        barMaxWidth: 28,
        itemStyle: { color: palette.brand, borderRadius: [4, 4, 0, 0] },
      },
      {
        name: revenue,
        type: "line",
        yAxisIndex: 1,
        data: series.revenue,
        symbol: "circle",
        symbolSize: 6,
        itemStyle: { color: palette.contrast },
        lineStyle: { color: palette.contrast, width: 1.5 },
      },
    ],
  };
}

export function dailySeries(items: DayPoint[] = []) {
  return {
    categories: items.map((item) => item.day),
    requests: items.map((item) => item.requests ?? 0),
    revenue: items.map((item) => item.revenue_minor ?? 0),
  };
}

export function dailyChartOption(items: DayPoint[], labels: ChartLabels = {}, palette: ChartPalette = chartPalette()) {
  const series = dailySeries(items);
  const requests = labels.requests || "requests";
  const revenue = labels.revenue || "revenue";
  return {
    color: [palette.brand, palette.contrast],
    tooltip: tooltip(palette),
    legend: { data: [requests, revenue], textStyle: axisLabel(palette), top: 0 },
    grid: { left: 48, right: 52, top: 36, bottom: 28, containLabel: true },
    xAxis: {
      type: "category",
      data: series.categories,
      axisLabel: axisLabel(palette),
      axisLine: { lineStyle: { color: palette.hairline } },
      axisTick: { show: false },
    },
    yAxis: [valueAxis(palette, requests), { ...valueAxis(palette, revenue), splitLine: { show: false } }],
    series: [
      {
        name: requests,
        type: "line",
        data: series.requests,
        symbol: "circle",
        symbolSize: 6,
        itemStyle: { color: palette.brand },
        lineStyle: { color: palette.brand, width: 1.5 },
      },
      {
        name: revenue,
        type: "line",
        yAxisIndex: 1,
        data: series.revenue,
        symbol: "circle",
        symbolSize: 6,
        itemStyle: { color: palette.contrast },
        lineStyle: { color: palette.contrast, width: 1.5 },
      },
    ],
  };
}
