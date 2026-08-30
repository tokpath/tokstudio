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

export const userSections: NavItem[] = [
  { href: "#wallet", label: "现金钱包" },
  { href: "#plans", label: "套餐与权益" },
  { href: "#keys", label: "API Key" },
  { href: "#examples", label: "接入示例" },
  { href: "#usage", label: "用量与账单" },
  { href: "#media", label: "媒体任务" },
  { href: "#settings", label: "个人设置" },
];

export const channelSections: NavItem[] = [
  { href: "#users", label: "本渠道用户" },
  { href: "#plans", label: "本渠道套餐" },
  { href: "#promos", label: "推广链接" },
  { href: "#attribution", label: "本渠道归因" },
  { href: "#usage", label: "本渠道用量" },
  { href: "#settlements", label: "本渠道结算" },
  { href: "#commissions", label: "渠道额度与佣金" },
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
