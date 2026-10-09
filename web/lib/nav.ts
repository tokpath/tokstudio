export type NavItem = { href: string; key: string; hint?: string; unavailable?: boolean };

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
  { titleKey: "start", items: [{ href: "/app/keys", key: "keys" }, { href: "/app/catalog", key: "catalog" }] },
  { titleKey: "ledger", items: [{ href: "/app/usage", key: "usage" }, { href: "/app/wallet", key: "wallet" }] },
  { titleKey: "people", items: [{ href: "/app/referral", key: "referral" }] },
  { titleKey: "more", items: [{ href: "/app/settings", key: "settings" }] },
  { titleKey: "tools", items: [{ href: "/app/playground", key: "playground" }, { href: "/app/media", key: "media" }] },
];

/** 设置子页（ofox 用户菜单）；侧栏只高亮「设置」。未上线的入口只标注、不当成可用功能。 */
export const userSettingsNav: NavItem[] = [
  { href: "/app/settings", key: "account" },
  { href: "/app/settings/team", key: "team", unavailable: true },
  { href: "/app/settings/members", key: "members", unavailable: true },
  { href: "/app/settings/billing", key: "billing" },
  { href: "/app/settings/quotas", key: "quotas" },
  { href: "/app/settings/apps", key: "apps", unavailable: true },
  { href: "/app/settings/webhooks", key: "webhooks", unavailable: true },
];

export const userSections: NavItem[] = userNavGroups.flatMap((group) => group.items);

/** 控制台侧栏当前项：门户根路径只精确匹配，避免点亮所有子页。 */
export function isNavActive(pathname: string, href: string) {
  if (href === "/app" || href === "/channel" || href === "/partner" || href === "/admin") {
    return pathname === href || pathname === `${href}/`;
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export function availableNavItems(items: NavItem[]) {
  return items.filter((item) => !item.unavailable);
}

/** 当前路径对应的导航项：更长的 href 优先（设置子页盖过「设置」）。 */
export function navItemForPath(pathname: string, items: NavItem[]) {
  return items
    .filter((item) => isNavActive(pathname, item.href))
    .sort((a, b) => b.href.length - a.href.length)[0];
}

export function consoleItemHref(item: { href: string }, hashPrefix: string) {
  if (item.href.startsWith("/")) {
    return item.href;
  }
  return `${hashPrefix}${item.href}`;
}

export const channelNavGroups: { titleKey: string; items: NavItem[] }[] = [
  { titleKey: "business", items: [{ href: "/channel", key: "overview" }] },
  { titleKey: "operations", items: [{ href: "/channel/users", key: "users" }] },
  { titleKey: "scope", items: [{ href: "/channel/promos", key: "promos" }] },
  { titleKey: "models", items: [{ href: "/channel/models", key: "models" }] },
  { titleKey: "ledger", items: [{ href: "/channel/commissions", key: "commissions" }, { href: "/channel/settlements", key: "settlements" }] },
];

export function channelNavGroupsFor(channelType?: string) {
  return channelType === "C" ? oemNavGroups : channelNavGroups;
}

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
  { titleKey: "groupOverview", items: [{ href: "/admin", key: "overview" }] },
  { titleKey: "groupCustomers", items: [{ href: "/admin/users", key: "users" }, { href: "/admin/channels", key: "channels" }, { href: "/admin/promos", key: "promos" }] },
  { titleKey: "groupCatalog", items: [{ href: "/admin/models", key: "models" }, { href: "/admin/plans", key: "plans" }, { href: "/admin/providers", key: "providers" }, { href: "/admin/routes", key: "routes" }] },
  { titleKey: "groupFinance", items: [{ href: "/admin/payments", key: "payments" }, { href: "/admin/commission", key: "commission" }, { href: "/admin/billing", key: "billing" }] },
  { titleKey: "groupExceptions", items: [{ href: "/admin/reconciliation", key: "reconciliation" }, { href: "/admin/usage", key: "usage" }, { href: "/admin/media", key: "media" }, { href: "/admin/alerts", key: "alerts" }] },
  { titleKey: "groupReports", items: [{ href: "/admin/metrics", key: "metrics" }, { href: "/admin/margin", key: "margin" }] },
  { titleKey: "groupSettings", items: [{ href: "/admin/brands", key: "brands" }, { href: "/admin/staff", key: "staff" }, { href: "/admin/audit", key: "audit" }, { href: "/admin/settings", key: "settings" }] },
];
export const adminNavKeys = adminGroups.flatMap(group => group.items.map(item => item.key));

/** OEM tasks are explicit and independent from platform technical administration. */
export const oemNavGroups: { titleKey: string; items: NavItem[] }[] = [
  { titleKey: "groupOverview", items: [{ href: "/channel", key: "overview" }] },
  { titleKey: "groupCustomers", items: [{ href: "/channel/users", key: "users" }, { href: "/channel/subchannels", key: "channels" }, { href: "/channel/promos", key: "promos" }] },
  { titleKey: "groupCatalog", items: [{ href: "/channel/models", key: "models" }, { href: "/channel/plans", key: "plans" }] },
  { titleKey: "groupFinance", items: [{ href: "/channel/payments", key: "payments" }, { href: "/channel/commission", key: "commission" }, { href: "/channel/ledger", key: "billing" }] },
  { titleKey: "groupExceptions", items: [{ href: "/channel/alerts", key: "alerts" }, { href: "/channel/usage", key: "usage" }, { href: "/channel/reconciliation", key: "reconciliation" }, { href: "/channel/media", key: "media" }] },
  { titleKey: "groupReports", items: [{ href: "/channel/metrics", key: "metrics" }, { href: "/channel/margin", key: "margin" }] },
  { titleKey: "groupTeam", items: [{ href: "/channel/brand", key: "brand" }, { href: "/channel/staff", key: "staff" }, { href: "/channel/settings", key: "settings" }, { href: "/channel/audit", key: "audit" }] },
];

/** Secondary tools belong to their parent page instead of a second OEM sidebar. */
export function channelNavItemForPath(pathname: string, channelType?: string) {
  if (channelType === "C") {
    const parent = [
            ["/channel/keys", "/channel/users"],
            ["/channel/rules", "/channel/commission"],
      ["/channel/commissions", "/channel/ledger"],
      ["/channel/settlements", "/channel/commission"],
      ["/channel/attribution", "/channel/promos"],
    ].find(([prefix]) => pathname === prefix || pathname.startsWith(`${prefix}/`));
    if (parent) pathname = parent[1];
  }
  return navItemForPath(pathname, channelNavGroupsFor(channelType).flatMap((group) => group.items));
}

export function isConsolePath(pathname: string) {
  return (
    pathname.startsWith("/app") ||
    pathname.startsWith("/console") ||
    pathname.startsWith("/channel") ||
    pathname.startsWith("/partner") ||
    pathname.startsWith("/admin")
  );
}

/** 登录页和控制台入口不套公共顶栏/页脚。 */
export function isAuthPath(pathname: string) {
  return (
    pathname === "/login" ||
    pathname.startsWith("/login/") ||
    pathname === "/enter" ||
    pathname.startsWith("/enter/")
  );
}
