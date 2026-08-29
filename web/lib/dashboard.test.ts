import { describe, expect, it } from "vitest";
import { formatDashboard } from "./dashboard";

describe("formatDashboard", () => {
  it("summarizes revenue margin and pending", () => {
    expect(
      formatDashboard({
        totals: { revenue_minor: 10, gross_profit_minor: 4, pending_reconciliation_count: 1 },
        alerts: [{ kind: "pending_reconciliation" }],
      }),
    ).toContain("待对账 1");
  });
});
