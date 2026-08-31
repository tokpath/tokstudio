export type NavItem = { href: string; key: string; hint?: string };

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
  { href: "/models", key: "models" },
  { href: "/docs", key: "docs" },
  { href: "/enterprise", key: "enterprise" },
] as const;

export const PUBLIC_NAV_MORE = [
  { href: "/quickstart", key: "quickstart" },
  { href: "/best-value", key: "bestValue" },
  { href: "/model-finder", key: "finder" },
  { href: "/vibe-coding", key: "vibe" },
  { href: "/trust", key: "trustCenter" },
] as const;

/** 用户台侧栏分组：ofox 登录后 IA + DESIGN.md 账本入口。 */
export const userNavGroups: { titleKey: string; items: NavItem[] }[] = [
  {
    titleKey: "start",
    items: [
      { href: "/app", key: "overview" },
      { href: "/app/playground", key: "playground" },
      { href: "/app/keys", key: "keys" },
      { href: "/app/catalog", key: "catalog" },
    ],
  },
  {
    titleKey: "ledger",
    items: [
      { href: "/app/wallet", key: "wallet" },
      { href: "/app/plans", key: "plans" },
      { href: "/app/usage", key: "usage" },
      { href: "/app/activity", key: "activity" },
      { href: "/app/media", key: "media" },
    ],
  },
  {
    titleKey: "people",
    items: [{ href: "/app/referral", key: "referral" }],
  },
  {
    titleKey: "more",
    items: [
      { href: "/app/docs", key: "docs" },
      { href: "/app/settings", key: "settings" },
    ],
  },
];

/** 设置子页（ofox 用户菜单）；侧栏只高亮「设置」。 */
export const userSettingsNav: NavItem[] = [
  { href: "/app/settings", key: "account" },
  { href: "/app/settings/team", key: "team" },
  { href: "/app/settings/members", key: "members" },
  { href: "/app/settings/billing", key: "billing" },
  { href: "/app/settings/quotas", key: "quotas" },
  { href: "/app/settings/apps", key: "apps" },
  { href: "/app/settings/webhooks", key: "webhooks" },
];

export const userSections: NavItem[] = userNavGroups.flatMap((group) => group.items);

/** 控制台侧栏当前项：门户根路径只精确匹配，避免点亮所有子页。 */
export function isNavActive(pathname: string, href: string) {
  if (href === "/app" || href === "/channel" || href === "/partner" || href === "/admin") {
    return pathname === href || pathname === `${href}/`;
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export function consoleItemHref(item: { href: string }, hashPrefix: string) {
  if (item.href.startsWith("/")) {
    return item.href;
  }
  return `${hashPrefix}${item.href}`;
}

export const channelNavGroups: { titleKey: string; items: NavItem[] }[] = [
  {
    titleKey: "channel",
    items: [
      { href: "/channel", key: "overview" },
      { href: "/channel/users", key: "users" },
      { href: "/channel/plans", key: "plans" },
      { href: "/channel/promos", key: "promos" },
    ],
  },
  {
    titleKey: "ledger",
    items: [
      { href: "/channel/attribution", key: "attribution" },
      { href: "/channel/usage", key: "usage" },
      { href: "/channel/settlements", key: "settlements" },
      { href: "/channel/commissions", key: "commissions" },
    ],
  },
];

export const partnerNavGroups: { titleKey: string; items: NavItem[] }[] = [
  {
    titleKey: "scope",
    items: [
      { href: "/partner", key: "hierarchy" },
      { href: "/partner/users", key: "users" },
      { href: "/partner/commissions", key: "commissions" },
      { href: "/partner/settlements", key: "settlements" },
    ],
  },
];

export const channelSections: NavItem[] = channelNavGroups.flatMap((group) => group.items);

export const partnerSections: NavItem[] = partnerNavGroups.flatMap((group) => group.items);

export const adminGroups: { titleKey: string; items: { href: string; key: string }[] }[] = [
  {
    titleKey: "groupOverview",
    items: [{ href: "/admin", key: "overview" }],
  },
  {
    titleKey: "groupCatalog",
    items: [
      { href: "/admin/providers", key: "providers" },
      { href: "/admin/models", key: "models" },
      { href: "/admin/routes", key: "routes" },
      { href: "/admin/keys", key: "keys" },
    ],
  },
  {
    titleKey: "groupBilling",
    items: [
      { href: "/admin/plans", key: "plans" },
      { href: "/admin/prices", key: "prices" },
      { href: "/admin/payments", key: "payments" },
      { href: "/admin/billing", key: "billing" },
      { href: "/admin/usage", key: "usage" },
    ],
  },
  {
    titleKey: "groupDistribution",
    items: [
      { href: "/admin/channels", key: "channels" },
      { href: "/admin/promos", key: "promos" },
      { href: "/admin/commission", key: "commission" },
    ],
  },
  {
    titleKey: "groupOps",
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
  return (
    pathname.startsWith("/app") ||
    pathname.startsWith("/console") ||
    pathname.startsWith("/channel") ||
    pathname.startsWith("/partner") ||
    pathname.startsWith("/admin")
  );
}
