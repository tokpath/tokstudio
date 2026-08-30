export type MetricPoint = { key: string; requests?: number; revenue_minor?: number; success_rate?: number };
export type DayPoint = { day: string; requests?: number; revenue_minor?: number; success_rate?: number };

const BRAND = "#2150D6";
const MUTE = "#8A8378";
const CONTRAST = "#4A453C";

export function dashboardSeries(items: MetricPoint[] = []) {
  return {
    categories: items.map((item) => item.key),
    requests: items.map((item) => item.requests ?? 0),
    revenue: items.map((item) => item.revenue_minor ?? 0),
    success: items.map((item) => Number(((item.success_rate ?? 0) * 100).toFixed(1))),
  };
}

export function requestChartOption(items: MetricPoint[]) {
  const series = dashboardSeries(items);
  return {
    tooltip: { trigger: "axis" },
    xAxis: { type: "category", data: series.categories, axisLabel: { color: MUTE } },
    yAxis: { type: "value", axisLabel: { color: MUTE } },
    series: [
      { name: "requests", type: "bar", data: series.requests, itemStyle: { color: BRAND } },
      { name: "revenue", type: "line", data: series.revenue, itemStyle: { color: CONTRAST } },
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

export function dailyChartOption(items: DayPoint[]) {
  const series = dailySeries(items);
  return {
    tooltip: { trigger: "axis" },
    xAxis: { type: "category", data: series.categories, axisLabel: { color: MUTE } },
    yAxis: { type: "value", axisLabel: { color: MUTE } },
    series: [
      { name: "requests", type: "line", data: series.requests, itemStyle: { color: BRAND } },
      { name: "revenue", type: "line", data: series.revenue, itemStyle: { color: CONTRAST } },
    ],
  };
}
