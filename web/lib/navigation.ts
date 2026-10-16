export const customerNav = [
  ["", "资产总览"], ["plans", "套餐与续费"], ["orders", "购买记录"], ["keys", "API Key"], ["requests", "用量与账单"], ["playground", "模型体验"], ["referrals", "邀请与资格"], ["commissions", "佣金明细"], ["activities", "KOL 活动"], ["withdrawals", "佣金提现"], ["refunds", "USD 退款"], ["invoices", "开票与凭证"], ["notifications", "通知"], ["account", "安全与账号"],
] as const;
export const operatorNav = [
  { group: "经营", items: [["overview", "经营概览"], ["customers", "客户服务"], ["plans", "套餐与续费"], ["orders", "订单与交付"], ["payments", "收款与分配"], ["evidence", "私有金融凭据"], ["payment-methods", "收款配置"], ["pricing", "模型与定价"], ["catalog-changes", "调价与模型通知"], ["valuations", "收款估值依据"], ["receipt-valuations", "已收款估值待办"], ["gifts","赠送与补偿"], ["marketing", "活动与奖励"], ["commissions", "资格与分佣"]] },
  { group: "资金与权益", items: [["assets", "四类资产"], ["ledger", "账本与占用"], ["refunds", "退款与冲正"], ["withdrawals", "提现办理"], ["disputes", "争议与扣回"], ["invoices", "票据办理"], ["reconciliation", "对账与差额"], ["reports", "经营报表"], ["exports", "私有导出"]] },
  { group: "合作", items: [["operators", "OEM 与渠道"], ["agreements", "供货与授权"], ["procurement", "采购与补货"], ["procurement-invoices", "采购开票"], ["coverage", "覆盖与超卖"], ["alerts", "风险预警"], ["takeovers", "渠道接管"], ["takeover-settlements", "接管资金结清"], ["exits", "合作退出"]] },
  { group: "设置", items: [["branding", "品牌主题"], ["domains", "品牌域名"], ["channel-themes", "渠道外观"], ["staff", "人员与权限"], ["approvals", "独立复核"], ["risk", "风险阈值"], ["restrictions", "账号与业务限制"], ["notifications", "通知与授权"], ["business-status", "营业与配置检查"], ["audit", "审计记录"], ["retention", "数据保留"]] },
  { group: "平台技术", items: [["payment-assets", "支付资产与网络"], ["providers", "供应商与密钥"], ["catalog", "公开模型与能力"], ["routes", "上游路由与参数"], ["routing", "动态选路与熔断"], ["requests", "请求与成本证据"], ["jobs", "任务与异常恢复"], ["mail", "验证与通知邮件"]] },
] as const;
export function resourceLabel(resource: string) { for (const group of operatorNav) for (const item of group.items) if (item[0] === resource) return item[1]; return resource; }
