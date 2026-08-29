import { describe, expect, it } from "vitest";
import { dashboardSeries, requestChartOption } from "./charts";

describe("dashboardSeries", () => {
  it("builds echarts categories from metrics", () => {
    const series = dashboardSeries([{ key: "tokenhub/echo-1", requests: 3, revenue_minor: 12, success_rate: 1 }]);
    expect(series.categories).toEqual(["tokenhub/echo-1"]);
    expect(series.requests).toEqual([3]);
    expect(requestChartOption([{ key: "echo", requests: 1 }]).series[0].type).toBe("bar");
  });
});
