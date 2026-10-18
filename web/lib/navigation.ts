export type NavigationItem = readonly [resource: string, label: string];
export type NavigationSection = { id: string; group: string; items: readonly NavigationItem[] };

// Routes remain addressable; the first level represents tasks, and the active
// task exposes its own secondary navigation. No business capability is removed.
export const customerSections: readonly NavigationSection[] = [
  { id: "overview", group: "概览", items: [["", "概览"]] },
  { id: "models", group: "模型与接入", items: [["models", "模型与接入"], ["playground", "在线体验"]] },
  { id: "keys", group: "API Key", items: [["keys", "API Key"]] },
  { id: "usage", group: "用量与账单", items: [["requests", "调用账单"]] },
  { id: "funds", group: "套餐与资金", items: [["plans", "套餐与续费"], ["orders", "购买记录"], ["credit-sources", "额度来源"], ["topups", "USD 充值"], ["refunds", "支付余额退款"], ["invoices", "开票与凭证"]] },
  { id: "promotion", group: "邀请与收益", items: [["referrals", "邀请与资格"], ["commissions", "佣金明细"], ["activities", "我的推广活动"], ["withdrawals", "佣金提现"]] },
  { id: "account", group: "账号设置", items: [["account", "账号与安全"], ["notifications", "通知与订阅"]] },
];
export const customerNav = customerSections.map(section => section.items[0]);
export const operatorNav: readonly NavigationSection[] = [
  { id: "overview", group: "工作台", items: [["overview", "工作台"]] },
  { id: "customers", group: "客户", items: [["customers", "客户服务"]] },
  { id: "products", group: "商品与定价", items: [["plans", "套餐与续费"], ["pricing", "模型与定价"], ["catalog-changes", "价格与模型变更"]] },
  { id: "marketing", group: "营销与推广", items: [["marketing", "活动管理"], ["commissions", "邀请与分佣规则"], ["gifts", "赠送与补偿"]] },
  { id: "finance", group: "资金办理", items: [["orders", "订单与交付"], ["payments", "收款与分配"], ["receipt-valuations", "收款待估值"], ["refunds", "退款与冲正"], ["withdrawals", "提现办理"], ["disputes", "争议与扣回"], ["invoices", "票据办理"], ["reconciliation", "对账与差额"], ["assets", "资产账户"], ["ledger", "账本与占用"], ["valuations", "估值依据"], ["evidence", "付款凭据"]] },
  { id: "cooperation", group: "采购与合作", items: [["procurement", "采购与补货"], ["operators", "下属经营方"], ["agreements", "供货关系"], ["contracts", "合作合同"], ["coverage", "履约与超卖"], ["alerts", "风险预警"], ["procurement-invoices", "采购开票"], ["takeovers", "渠道接管"], ["takeover-settlements", "接管结清"], ["exits", "合作退出"], ["branding", "品牌外观"], ["domains", "品牌域名"], ["channel-themes", "渠道外观"]] },
  { id: "reports", group: "报表", items: [["reports", "经营报表"], ["exports", "导出记录"]] },
  { id: "settings", group: "设置", items: [["business-status", "营业设置"], ["payment-methods", "收款方式"], ["staff", "人员与权限"], ["approvals", "复核办理"], ["risk", "风险阈值"], ["restrictions", "业务限制"], ["notifications", "通知管理"], ["audit", "操作审计"], ["retention", "数据保留"]] },
  { id: "technology", group: "平台技术", items: [["catalog", "公开模型"], ["providers", "模型供应商"], ["routes", "上游路由"], ["routing", "选路与熔断"], ["requests", "调用与成本核实"], ["jobs", "任务恢复"], ["payment-assets", "支付资产与网络"], ["mail", "邮件服务"]] },
];
export const operatorSections = operatorNav;
const platformOnly = new Set(["catalog", "providers", "routes", "routing", "requests", "jobs", "mail", "payment-assets"]);
const parentOnly = new Set(["operators", "channel-themes", "domains", "takeovers", "takeover-settlements"]);
type Workspace = { operator: { kind: string }; permissions: readonly string[] };
export function canReadResource(workspace: Workspace | undefined, resource: string): boolean {
  if (!workspace) return false;
  const base = resource.split("/")[0];
  if (platformOnly.has(base) && workspace.operator.kind !== "platform") return false;
  if (parentOnly.has(base) && workspace.operator.kind === "channel") return false;
  return workspace.permissions.includes("*") || workspace.permissions.includes(`${base}.read`);
}
export function visibleOperatorSections(workspace: Workspace): NavigationSection[] {
  return operatorNav.map(section => ({ ...section, items: section.items.filter(([resource]) => canReadResource(workspace, resource)) })).filter(section => section.items.length > 0);
}
export function operatorLanding(workspace: Workspace & { operator: { id: string; kind: string } }): string {
  const first = visibleOperatorSections(workspace)[0]?.items[0]?.[0];
  return `/manage/${encodeURIComponent(workspace.operator.id)}/${first || "overview"}`;
}
export function sectionForResource(sections: readonly NavigationSection[], resource: string): NavigationSection | undefined {
  const base = resource.split("/")[0];
  return sections.find(section => section.items.some(([item]) => item === base));
}
export function resourceLabel(resource: string): string {
  const base = resource.split("/")[0];
  for (const section of operatorNav) for (const [key, label] of section.items) if (key === base) return label;
  for (const section of customerSections) for (const [key, label] of section.items) if (key === base) return label;
  return base;
}
