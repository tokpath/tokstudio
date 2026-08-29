export type MetricPoint = { key: string; requests?: number; revenue_minor?: number; success_rate?: number };

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
    xAxis: { type: "category", data: series.categories, axisLabel: { color: "#94a3b8" } },
    yAxis: { type: "value", axisLabel: { color: "#94a3b8" } },
    series: [
      { name: "requests", type: "bar", data: series.requests, itemStyle: { color: "#22d3ee" } },
      { name: "revenue", type: "line", data: series.revenue, itemStyle: { color: "#f59e0b" } },
    ],
  };
}
