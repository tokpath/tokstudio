import { describe, expect, it } from "vitest";
import { dashboardHero, formatDashboard } from "./dashboard";

describe("formatDashboard", () => {
  it("summarizes revenue margin and pending", () => {
    expect(
      formatDashboard({
        totals: { revenue_minor: 10, gross_profit_minor: 4, pending_reconciliation_count: 1, success_rate: 1, low_balance_wallets: 2 },
        alerts: [{ kind: "pending_reconciliation" }],
      }),
    ).toContain("待对账 1");
  });

  it("exposes DESIGN.md admin hero stats", () => {
    const cards = dashboardHero({
      totals: { pending_reconciliation_count: 2, gross_profit_minor: 4_000_000, commission_liability_minor: 500_000 },
      alerts: [{ kind: "provider_circuit_open" }],
    });
    expect(cards.map((c) => c.key)).toEqual(["heroPending", "heroProfit", "heroCommission", "heroHealth"]);
    expect(cards[0].v).toBe("2");
    expect(cards[1].v).toBe("$4.00");
    expect(cards[3].v).toBe("DEGRADED");
  });
});
