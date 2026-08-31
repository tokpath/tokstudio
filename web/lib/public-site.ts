/** ofox.ai 公开站信息架构 → TokenHub 路由映射（视觉走 DESIGN.md）。 */

export type PublicPageSpec = {
  href: string;
  ofox: string;
  label: string;
  purpose: string;
  auth?: boolean;
};

/** 验收用页面清单：每条都要有对应路由与完整区块。 */
export const PUBLIC_PAGE_SPECS: PublicPageSpec[] = [
  { href: "/", ofox: "https://ofox.ai/zh", label: "首页", purpose: "价目+接入示例+能力分区" },
  { href: "/models", ofox: "https://ofox.ai/zh/models", label: "模型目录", purpose: "搜索/筛选/价目表" },
  { href: "/quickstart", ofox: "https://ofox.ai/zh/quickstart", label: "快速开始", purpose: "三步接入" },
  { href: "/docs", ofox: "https://ofox.ai/docs", label: "文档", purpose: "协议与示例" },
  { href: "/docs/integrations", ofox: "https://ofox.ai/docs/integrations", label: "集成", purpose: "工具接入片段" },
  { href: "/docs/develop", ofox: "https://ofox.ai/docs/develop", label: "开发指南", purpose: "错误与安全" },
  { href: "/docs/changelog", ofox: "https://ofox.ai/docs/changelog", label: "更新日志", purpose: "版本记录" },
  { href: "/enterprise", ofox: "https://ofox.ai/zh/enterprise", label: "企业", purpose: "团队能力与对比" },
  { href: "/trust", ofox: "https://ofox.ai/zh/trust", label: "信任中心", purpose: "安全与留存说明" },
  { href: "/trust/subprocessors", ofox: "https://ofox.ai/zh/trust/subprocessors", label: "第三方服务商", purpose: "子处理商列表" },
  { href: "/best-value", ofox: "https://ofox.ai/zh/best-value", label: "性价比模型", purpose: "折扣价目" },
  { href: "/model-finder", ofox: "https://ofox.ai/zh/model-finder", label: "模型推荐器", purpose: "场景问卷推荐" },
  { href: "/vibe-coding", ofox: "https://ofox.ai/zh/vibe-coding", label: "Vibe Coding", purpose: "编程工具接入" },
  { href: "/video", ofox: "https://ofox.ai/zh/video", label: "视频模型", purpose: "视频能力落地页" },
  { href: "/image", ofox: "https://ofox.ai/zh/image", label: "图像模型", purpose: "图像能力落地页" },
  { href: "/leaderboards/models", ofox: "https://ofox.ai/zh/leaderboards/models", label: "模型排行", purpose: "用量份额榜" },
  { href: "/leaderboards/apps", ofox: "https://ofox.ai/zh/leaderboards/apps", label: "应用排行", purpose: "工具用量榜" },
  { href: "/leaderboards/labs", ofox: "https://ofox.ai/zh/leaderboards/labs", label: "厂商排行", purpose: "实验室份额榜" },
  { href: "/vs/openrouter", ofox: "https://ofox.ai/zh/vs/openrouter", label: "对比聚合网关", purpose: "竞品对照表" },
  { href: "/compare", ofox: "https://ofox.ai/compare", label: "模型对比", purpose: "两模型并排价目" },
  { href: "/promo", ofox: "https://ofox.ai/zh/partner", label: "合作推广", purpose: "渠道合作" },
  { href: "/promo/august", ofox: "https://ofox.ai/zh/promo/august", label: "充值优惠", purpose: "兑换码说明" },
  { href: "/desktop", ofox: "https://ofox.ai/zh/desktop", label: "Desktop", purpose: "桌面客户端说明" },
  { href: "/verify", ofox: "https://ofox.ai/verify/", label: "模型验真", purpose: "验真说明" },
  { href: "/awesome-ofox", ofox: "https://ofox.ai/zh/awesome-ofox", label: "生态应用", purpose: "Works with 墙" },
  { href: "/pricing", ofox: "https://ofox.ai/pricing", label: "定价", purpose: "按量付费说明" },
  { href: "/blog", ofox: "https://ofox.ai/zh/blog", label: "博客", purpose: "文章列表" },
  { href: "/terms", ofox: "https://ofox.ai/zh/terms-of-service", label: "服务条款", purpose: "法律条款" },
  { href: "/privacy", ofox: "https://ofox.ai/zh/privacy", label: "隐私政策", purpose: "隐私说明" },
  { href: "/login", ofox: "https://app.ofox.ai/auth", label: "登录", purpose: "邮箱/OTP/Google" },
  { href: "/app", ofox: "https://app.ofox.ai/dashboard", label: "总览", purpose: "余额/Key/回单", auth: true },
  { href: "/app/playground", ofox: "https://app.ofox.ai/playground", label: "快速试用", purpose: "选模型发一条", auth: true },
  { href: "/app/keys", ofox: "https://app.ofox.ai/manage/api-keys", label: "API Key", purpose: "创建/轮换/白名单", auth: true },
  { href: "/app/catalog", ofox: "https://app.ofox.ai/models", label: "模型广场", purpose: "控制台价目", auth: true },
  { href: "/app/usage", ofox: "https://app.ofox.ai/analytics", label: "用量与账单", purpose: "usage/账本", auth: true },
  { href: "/app/activity", ofox: "https://app.ofox.ai/analytics/activity", label: "请求明细", purpose: "路由回单表", auth: true },
  { href: "/app/wallet", ofox: "https://app.ofox.ai/manage/wallet", label: "余额/充值", purpose: "钱包与流水", auth: true },
  { href: "/app/plans", ofox: "https://app.ofox.ai/manage/wallet", label: "套餐", purpose: "订阅与权益", auth: true },
  { href: "/app/media", ofox: "https://app.ofox.ai/playground", label: "媒体任务", purpose: "视频/图像任务", auth: true },
  { href: "/app/referral", ofox: "https://app.ofox.ai/manage/referral", label: "推荐计划", purpose: "推荐码", auth: true },
  { href: "/app/docs", ofox: "https://ofox.ai/docs", label: "接入文档", purpose: "控制台示例", auth: true },
  { href: "/app/settings", ofox: "https://app.ofox.ai/settings/account", label: "账户", purpose: "资料与密码", auth: true },
  { href: "/app/settings/team", ofox: "https://app.ofox.ai/settings/organization", label: "团队", purpose: "结算主体", auth: true },
  { href: "/app/settings/members", ofox: "https://app.ofox.ai/settings/organization/members", label: "成员", purpose: "邀请", auth: true },
  { href: "/app/settings/billing", ofox: "https://app.ofox.ai/settings/organization/billing-profile", label: "开票资料", purpose: "抬头税号", auth: true },
  { href: "/app/settings/quotas", ofox: "https://app.ofox.ai/settings/quotas", label: "用量配额", purpose: "额度汇总", auth: true },
  { href: "/app/settings/apps", ofox: "https://app.ofox.ai/settings/connected-apps", label: "已连接应用", purpose: "OAuth 授权", auth: true },
  { href: "/app/settings/webhooks", ofox: "https://app.ofox.ai/settings/webhooks", label: "Webhook", purpose: "回调地址", auth: true },
];

export const FOOTER_GROUPS = [
  {
    titleKey: "product",
    links: [
      { href: "/models", labelKey: "models" },
      { href: "/quickstart", labelKey: "quickstart" },
      { href: "/desktop", labelKey: "desktop" },
      { href: "/vibe-coding", labelKey: "vibe" },
      { href: "/enterprise", labelKey: "enterprise" },
      { href: "/promo", labelKey: "promo" },
    ],
  },
  {
    titleKey: "tools",
    links: [
      { href: "/awesome-ofox", labelKey: "awesome" },
      { href: "/leaderboards/models", labelKey: "leaderboard" },
      { href: "/model-finder", labelKey: "finder" },
      { href: "/best-value", labelKey: "bestValue" },
      { href: "/verify", labelKey: "verify" },
      { href: "/vs/openrouter", labelKey: "vsOr" },
    ],
  },
  {
    titleKey: "resources",
    links: [
      { href: "/docs", labelKey: "docs" },
      { href: "/blog", labelKey: "blog" },
      { href: "/trust", labelKey: "trust" },
      { href: "/trust/subprocessors", labelKey: "subprocessors" },
      { href: "/terms", labelKey: "terms" },
      { href: "/privacy", labelKey: "privacy" },
    ],
  },
] as const;

export type PublicModel = {
  id?: string;
  vendor?: string;
  display_name?: string;
  capabilities?: Record<string, unknown>;
  sell_price?: Record<string, unknown>;
  status?: string;
};

export function formatPriceCell(price: Record<string, unknown> | undefined, key: string) {
  if (!price || price[key] == null || price[key] === "") return "—";
  const raw = String(price[key]);
  const n = Number(raw);
  if (!Number.isFinite(n)) return raw;
  // seed 里是 per-token；展示成 /M tokens 更像价目单
  if (n > 0 && n < 0.01) return `$${(n * 1_000_000).toFixed(2)}/M`;
  return `$${n}`;
}

export function capabilityLabels(caps?: Record<string, unknown>) {
  const supported = Array.isArray(caps?.supported_parameters)
    ? (caps?.supported_parameters as string[])
    : [];
  const map: Record<string, string> = {
    vision: "视觉",
    tools: "函数",
    reasoning: "推理",
    stream: "流式",
    json: "JSON",
  };
  const labels = supported.map((p) => map[p] || p).filter((v, i, a) => a.indexOf(v) === i);
  return labels.slice(0, 6);
}

export function modalityOf(model: PublicModel): "text" | "image" | "video" | "other" {
  const id = (model.id || "").toLowerCase();
  const name = (model.display_name || "").toLowerCase();
  if (id.includes("seedance") || id.includes("video") || name.includes("video")) return "video";
  if (id.includes("image") || name.includes("image") || name.includes("seedream")) return "image";
  return "text";
}
