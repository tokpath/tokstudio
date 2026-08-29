export function formatDashboard(dashboard: {
  totals?: { revenue_minor?: number; gross_profit_minor?: number; pending_reconciliation_count?: number };
  alerts?: { kind?: string }[];
}): string {
  const revenue = dashboard.totals?.revenue_minor ?? 0;
  const profit = dashboard.totals?.gross_profit_minor ?? 0;
  const pending = dashboard.totals?.pending_reconciliation_count ?? 0;
  const alerts = dashboard.alerts?.length ?? 0;
  return `收入 ${revenue} micro-USD，毛利 ${profit}，待对账 ${pending}，未关闭告警 ${alerts}`;
}
