import { formatUsdMinor } from "@/lib/money";

export type DashboardTotals = {
  revenue_minor?: number;
  gross_profit_minor?: number;
  commission_liability_minor?: number;
  pending_reconciliation_count?: number;
  success_rate?: number;
  low_balance_wallets?: number;
  timeouts?: number;
  prompt_tokens?: number;
  video_seconds?: number;
  upstream_errors?: number;
};

export type DashboardAlert = { kind?: string };

/** DESIGN.md 管理台英雄：待对账、毛利、佣金负债、Provider 健康。不是营销大英雄区。 */
export function dashboardHero(dashboard: { totals?: DashboardTotals; alerts?: DashboardAlert[] }) {
  const totals = dashboard.totals || {};
  const alerts = dashboard.alerts || [];
  const healthKnown = Array.isArray(dashboard.alerts);
  const circuit = alerts.some((a) => a.kind === "provider_circuit_open" || a.kind === "low_success_rate");
  return [
    { key: "heroPending", hintKey: "heroPendingHint", v: String(totals.pending_reconciliation_count ?? "—"), href: "/admin/reconciliation" },
    { key: "heroProfit", hintKey: "heroProfitHint", v: formatUsdMinor(totals.gross_profit_minor), href: undefined },
    { key: "heroCommission", hintKey: "heroCommissionHint", v: formatUsdMinor(totals.commission_liability_minor), href: undefined },
    { key: "heroHealth", hintKey: !healthKnown ? "heroHealthUnknown" : circuit ? "heroHealthBad" : "heroHealthOk", v: !healthKnown ? "UNKNOWN" : circuit ? "DEGRADED" : "READY", href: undefined },
  ];
}

export function dashboardSummaryParams(dashboard: {
  totals?: DashboardTotals;
  alerts?: DashboardAlert[];
}) {
  const revenue = dashboard.totals?.revenue_minor;
  const profit = dashboard.totals?.gross_profit_minor;
  const pending = dashboard.totals?.pending_reconciliation_count;
  const rate = dashboard.totals?.success_rate;
  const risk = dashboard.totals?.low_balance_wallets;
  const timeouts = dashboard.totals?.timeouts;
  const tokens = dashboard.totals?.prompt_tokens;
  const video = dashboard.totals?.video_seconds;
  const alerts = dashboard.alerts?.length;
  return {
    rate: rate === undefined ? "—" : (rate * 100).toFixed(0),
    revenue: formatUsdMinor(revenue),
    profit: formatUsdMinor(profit),
    pending: pending ?? "—",
    risk: risk ?? "—",
    timeouts: timeouts ?? "—",
    tokens: tokens ?? "—",
    video: video ?? "—",
    alerts: alerts ?? "—",
  };
}
