export type NavItem = { href: string; label: string; hint?: string };

export const portalLinks = [
  { href: "/", key: "public" as const },
  { href: "/docs", key: "docs" as const },
  { href: "/app", key: "app" as const },
  { href: "/channel", key: "channel" as const },
  { href: "/partner", key: "partner" as const },
  { href: "/admin", key: "admin" as const },
  { href: "/login", key: "login" as const },
];

/** 顶栏主链（兼容旧引用）；真实下拉见 `mega-nav.ts`。 */
export const PUBLIC_NAV = [
  { href: "/models", label: "模型" },
  { href: "/docs", label: "文档" },
  { href: "/enterprise", label: "企业" },
] as const;

export const PUBLIC_NAV_MORE = [
  { href: "/quickstart", label: "快速开始" },
  { href: "/best-value", label: "性价比" },
  { href: "/model-finder", label: "推荐器" },
  { href: "/vibe-coding", label: "Vibe Coding" },
  { href: "/trust", label: "信任中心" },
] as const;

/** 用户台侧栏分组：ofox 登录后 IA + DESIGN.md 账本入口。 */
export const userNavGroups: { title: string; items: NavItem[] }[] = [
  {
    title: "开始",
    items: [
      { href: "/app", label: "总览" },
      { href: "/app/playground", label: "快速试用" },
      { href: "/app/keys", label: "API Key" },
      { href: "/app/catalog", label: "模型广场" },
    ],
  },
  {
    title: "账本",
    items: [
      { href: "/app/wallet", label: "余额/充值" },
      { href: "/app/plans", label: "套餐" },
      { href: "/app/usage", label: "用量/账单" },
      { href: "/app/activity", label: "请求明细" },
      { href: "/app/media", label: "媒体任务" },
    ],
  },
  {
    title: "用户",
    items: [{ href: "/app/referral", label: "推荐计划" }],
  },
  {
    title: "更多",
    items: [
      { href: "/app/docs", label: "文档" },
      { href: "/app/settings", label: "设置" },
    ],
  },
];

/** 设置子页（ofox 用户菜单）；侧栏只高亮「设置」。 */
export const userSettingsNav: NavItem[] = [
  { href: "/app/settings", label: "账户" },
  { href: "/app/settings/team", label: "团队" },
  { href: "/app/settings/members", label: "成员" },
  { href: "/app/settings/billing", label: "开票资料" },
  { href: "/app/settings/quotas", label: "用量配额" },
  { href: "/app/settings/apps", label: "已连接应用" },
  { href: "/app/settings/webhooks", label: "Webhook" },
];

export const userSections: NavItem[] = userNavGroups.flatMap((group) => group.items);

/** 控制台侧栏当前项：总览只精确匹配，避免 /app 点亮所有子页。 */
export function isNavActive(pathname: string, href: string) {
  if (href === "/app") {
    return pathname === "/app" || pathname === "/app/";
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export function consoleItemHref(item: NavItem, hashPrefix: string) {
  if (item.href.startsWith("/")) {
    return item.href;
  }
  return `${hashPrefix}${item.href}`;
}

export const channelSections: NavItem[] = [
  { href: "#users", label: "本渠道用户" },
  { href: "#plans", label: "套餐" },
  { href: "#promos", label: "推广" },
  { href: "#attribution", label: "额度" },
  { href: "#usage", label: "用量" },
  { href: "#settlements", label: "结算" },
  { href: "#commissions", label: "佣金/结算" },
];

export const partnerSections: NavItem[] = [
  { href: "#scope", label: "我的层级" },
  { href: "#users", label: "范围内用户" },
  { href: "#commissions", label: "范围内佣金" },
  { href: "#settlements", label: "范围内结算" },
];

export const adminGroups: { title: string; items: { href: string; key: string }[] }[] = [
  {
    title: "总览",
    items: [{ href: "/admin", key: "overview" }],
  },
  {
    title: "目录与网关",
    items: [
      { href: "/admin/providers", key: "providers" },
      { href: "/admin/models", key: "models" },
      { href: "/admin/routes", key: "routes" },
      { href: "/admin/keys", key: "keys" },
    ],
  },
  {
    title: "账务",
    items: [
      { href: "/admin/plans", key: "plans" },
      { href: "/admin/prices", key: "prices" },
      { href: "/admin/payments", key: "payments" },
      { href: "/admin/billing", key: "billing" },
      { href: "/admin/usage", key: "usage" },
    ],
  },
  {
    title: "分销",
    items: [
      { href: "/admin/channels", key: "channels" },
      { href: "/admin/promos", key: "promos" },
      { href: "/admin/commission", key: "commission" },
    ],
  },
  {
    title: "运营",
    items: [
      { href: "/admin/metrics", key: "metrics" },
      { href: "/admin/media", key: "media" },
      { href: "/admin/users", key: "users" },
      { href: "/admin/alerts", key: "alerts" },
      { href: "/admin/runbooks", key: "runbooks" },
      { href: "/admin/audit", key: "audit" },
      { href: "/admin/settings", key: "settings" },
    ],
  },
];

export function isConsolePath(pathname: string) {
  return pathname.startsWith("/app") || pathname.startsWith("/channel") || pathname.startsWith("/partner") || pathname.startsWith("/admin");
}
