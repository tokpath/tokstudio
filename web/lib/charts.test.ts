import { describe, expect, it } from "vitest";
import { dailyChartOption, dailySeries, dashboardSeries, requestChartOption } from "./charts";

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
});
