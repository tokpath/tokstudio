export function formatDashboard(dashboard: {
  totals?: {
    revenue_minor?: number;
    gross_profit_minor?: number;
    pending_reconciliation_count?: number;
    success_rate?: number;
    low_balance_wallets?: number;
  };
  alerts?: { kind?: string }[];
}): string {
  const revenue = dashboard.totals?.revenue_minor ?? 0;
  const profit = dashboard.totals?.gross_profit_minor ?? 0;
  const pending = dashboard.totals?.pending_reconciliation_count ?? 0;
  const rate = dashboard.totals?.success_rate ?? 0;
  const risk = dashboard.totals?.low_balance_wallets ?? 0;
  const alerts = dashboard.alerts?.length ?? 0;
  return `成功率 ${(rate * 100).toFixed(0)}%，收入 ${revenue} micro-USD，毛利 ${profit}，待对账 ${pending}，低余额钱包 ${risk}，未关闭告警 ${alerts}`;
}
