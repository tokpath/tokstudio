import { describe, expect, it } from "vitest";
import { dashboardHero, dashboardSummaryParams } from "./dashboard";

describe("dashboardSummaryParams", () => {
  it("summarizes revenue margin and pending", () => {
    expect(
      dashboardSummaryParams({
        totals: { revenue_minor: 10, gross_profit_minor: 4, pending_reconciliation_count: 1, success_rate: 1, low_balance_wallets: 2 },
        alerts: [{ kind: "pending_reconciliation" }],
      }),
    ).toMatchObject({ pending: 1, revenue: 10, profit: 4, rate: "100", risk: 2, alerts: 1 });
  });

  it("exposes DESIGN.md admin hero stats", () => {
    const cards = dashboardHero({
      totals: { pending_reconciliation_count: 2, gross_profit_minor: 4_000_000, commission_liability_minor: 500_000 },
      alerts: [{ kind: "provider_circuit_open" }],
    });
    expect(cards.map((c) => c.key)).toEqual(["heroPending", "heroProfit", "heroCommission", "heroHealth"]);
    expect(cards[0].v).toBe("2");
    expect(cards[0].href).toBe("/admin/reconciliation");
    expect(cards[1].v).toBe("$4.00");
    expect(cards[3].v).toBe("DEGRADED");
  });
});
