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

function micro(n?: number) {
  if (n == null || !Number.isFinite(n)) return "—";
  return `$${(n / 1_000_000).toFixed(2)}`;
}

/** DESIGN.md 管理台英雄：待对账、毛利、佣金负债、Provider 健康。不是营销大英雄区。 */
export function dashboardHero(dashboard: { totals?: DashboardTotals; alerts?: DashboardAlert[] }) {
  const totals = dashboard.totals || {};
  const alerts = dashboard.alerts || [];
  const circuit = alerts.some((a) => a.kind === "provider_circuit_open" || a.kind === "low_success_rate");
  return [
    { key: "heroPending", hintKey: "heroPendingHint", v: String(totals.pending_reconciliation_count ?? "—") },
    { key: "heroProfit", hintKey: "heroProfitHint", v: micro(totals.gross_profit_minor) },
    { key: "heroCommission", hintKey: "heroCommissionHint", v: micro(totals.commission_liability_minor) },
    { key: "heroHealth", hintKey: circuit ? "heroHealthBad" : "heroHealthOk", v: circuit ? "DEGRADED" : "READY" },
  ];
}

export function formatDashboard(dashboard: {
  totals?: DashboardTotals;
  alerts?: DashboardAlert[];
}): string {
  const revenue = dashboard.totals?.revenue_minor ?? 0;
  const profit = dashboard.totals?.gross_profit_minor ?? 0;
  const pending = dashboard.totals?.pending_reconciliation_count ?? 0;
  const rate = dashboard.totals?.success_rate ?? 0;
  const risk = dashboard.totals?.low_balance_wallets ?? 0;
  const timeouts = dashboard.totals?.timeouts ?? 0;
  const tokens = dashboard.totals?.prompt_tokens ?? 0;
  const video = dashboard.totals?.video_seconds ?? 0;
  const alerts = dashboard.alerts?.length ?? 0;
  return `成功率 ${(rate * 100).toFixed(0)}%，收入 ${revenue} micro-USD，毛利 ${profit}，待对账 ${pending}，低余额钱包 ${risk}，超时 ${timeouts}，prompt ${tokens}，视频 ${video} 秒，未关闭告警 ${alerts}`;
}
