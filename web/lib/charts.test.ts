import { describe, expect, it } from "vitest";
import { dailyChartOption, dailySeries, dashboardSeries, requestChartOption, shortChartLabel } from "./charts";

describe("dashboardSeries", () => {
  it("builds echarts categories from metrics", () => {
    const series = dashboardSeries([{ key: "tokenhub/echo-1", requests: 3, revenue_minor: 12, success_rate: 1 }]);
    expect(series.categories).toEqual(["tokenhub/echo-1"]);
    expect(series.requests).toEqual([3]);
    expect(requestChartOption([{ key: "echo", requests: 1 }]).series[0].type).toBe("bar");
  });

  it("builds a daily line chart", () => {
    const series = dailySeries([{ day: "2026-08-29", requests: 4, revenue_minor: 10 }]);
    expect(series.categories).toEqual(["2026-08-29"]);
    expect(dailyChartOption([{ day: "2026-08-29", requests: 4 }]).series[0].type).toBe("line");
  });

  it("uses dual axes and no area fill", () => {
    const option = requestChartOption([{ key: "echo", requests: 2, revenue_minor: 80 }]);
    expect(option.yAxis).toHaveLength(2);
    expect(option.series[1].yAxisIndex).toBe(1);
    expect(option.series[0]).not.toHaveProperty("areaStyle");
    expect(option.grid.borderColor).toBeUndefined();
    expect(option.tooltip.borderWidth).toBe(1);
  });

  it("shortens long category labels", () => {
    expect(shortChartLabel("tokenhub/very-long-model-id", 12)).toBe("tokenhub/ve…");
  });
});
