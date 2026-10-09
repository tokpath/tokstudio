import { ADMIN_CONSOLE_ROLES, OEM_CONSOLE_ROLES } from "./console-home";

type NavGroupLike = { titleKey: string; items: { href: string; key: string }[] };

/** 登录与角色事实完成前不显示受保护的入口或写操作。 */
export type Viewer = {
  signedIn: boolean;
  loading: boolean;
  roles: string[];
  userId?: string;
  isPartner?: boolean;
  partnerRole?: string;
  channelType?: string;
  error?: boolean;
  partnerError?: boolean;
};

export const emptyViewer: Viewer = { signedIn: false, loading: true, roles: [] };

export type WriteAction =
  | "providers.write"
  | "providers.health"
  | "models.write"
  | "models.attach"
  | "models.grant"
  | "routes.write"
  | "keys.write"
  | "plans.write"
  | "plans.renew"
  | "prices.write"
  | "payments.write"
  | "billing.refund"
  | "billing.bonus"
  | "usage.replay"
  | "usage.resolve"
  | "margin.correct"
  | "users.write"
  | "channels.write"
  | "channels.quota"
  | "channels.payments.disable"
  | "partners.write"
  | "partners.view"
  | "brands.write"
  | "promos.write"
  | "commission.recovery.read"
  | "commission.write"
  | "audit.probe"
  | "audit.outbox"
  | "alerts.write"
  | "settings.totp"
  | "settings.thresholds"
  | "settings.thresholds.view"
  | "settings.canary"
  | "settings.circuit"
  | "settings.backup"
  | "settings.drill.payment"
  | "settings.drill.media"
  | "settings.drill.tls";

const P = "platform_admin";
const F = "finance_admin";
const O = "ops_admin";
const T = "tech_admin";
const A = "audit_readonly";

/** 管理台各页谁可以看见。财务不看上游密钥；技术不看退款/佣金写入口。 */
const ADMIN_PAGE_VIEW: Record<string, readonly string[]> = {
  "/admin": [P, F, O, T, A],
  "/admin/providers": [P, O, T, A],
  "/admin/models": [P, O, T, A],
  "/admin/routes": [P, O, T, A],
  "/admin/plans": [P, O, A],
  "/admin/prices": [P, F, O, A],
  "/admin/payments": [P, F, O, A],
  "/admin/billing": [P, F, O, A],
  "/admin/usage": [P, F, O, A],
  "/admin/margin": [P, F, O, A],
  "/admin/reconciliation": [P, F, O, A],
  "/admin/channels": [P, F, O, A],
  "/admin/partners": [P],
  "/admin/brands": [P, O, T, A],
  "/admin/oem-deliveries": [P,F,O,T,A],
  "/admin/promos": [P, O, A],
  "/admin/commission": [P, F, O, A],
  "/admin/metrics": [P, F, O, T, A],
  "/admin/media": [P, O, T, A],
  "/admin/users": [P],
  "/admin/staff": [P],
  "/admin/alerts": [P, O, T, A],
  "/admin/runbooks": [P, O, T, A],
  "/admin/audit": [P, A],
  "/admin/settings": [P, F, O, T],
};

const WRITE_ACTION_ROLES: Record<WriteAction, readonly string[]> = {
  "providers.write": [P, T],
  "providers.health": [P, T],
  "models.write": [P, O],
  "models.attach": [P, O, T],
  "models.grant": [P, O],
  "routes.write": [P, T],
  "keys.write": [P, T],
  "plans.write": [P, O],
  "plans.renew": [P],
  "prices.write": [P, F, O],
  "payments.write": [P, F],
  "billing.refund": [P, F],
  "billing.bonus": [P, F, O],
  "usage.replay": [P, F, O],
  "usage.resolve": [P, F, O],
  "margin.correct": [P, F, O],
  "users.write": [P],
  "channels.write": [P],
  "channels.quota": [P, F],
  "channels.payments.disable": [P, F, O],
  "partners.write": [P],
  "partners.view": [P],
  "brands.write": [P],
  "promos.write": [P],
  "commission.recovery.read": [P, F, A],
  "commission.write": [P, F],
  "audit.probe": [P],
  "audit.outbox": [P, T],
  "alerts.write": [P, O, T],
  "settings.totp": [P, F, O, T],
  "settings.thresholds": [P, O],
  "settings.thresholds.view": [P, O, T, A],
  "settings.canary": [P, O, T],
  "settings.circuit": [P, T],
  "settings.backup": [P, T],
  "settings.drill.payment": [P, F, T],
  "settings.drill.media": [P, T],
  "settings.drill.tls": [P, T],
};

export function shouldBypassRbac(viewer: Viewer): boolean {
  return false;
}

export function hasAnyRole(roles: string[] | undefined | null, allowed: readonly string[]): boolean {
  return Boolean(roles?.some((role) => allowed.includes(role)));
}

export function canAccessAdminConsole(roles: string[] | undefined | null): boolean {
  return hasAnyRole(roles, ADMIN_CONSOLE_ROLES);
}

export function canAccessChannelPortal(roles: string[] | undefined | null): boolean {
  return hasAnyRole(roles, [P, ...OEM_CONSOLE_ROLES]);
}

export type ChannelAction = "operations" | "finance" | "staff" | "paymentSettings";
export function canChannelAction(action: ChannelAction, viewer: Viewer): boolean {
  if (viewer.channelType === "B" && (action === "finance" || action === "paymentSettings")) return false;
  if (shouldBypassRbac(viewer) || viewer.roles.includes(P) || viewer.roles.includes("channel_admin")) return true;
  return action === "operations" ? viewer.roles.includes("oem_ops") : (action === "finance" || action === "paymentSettings") ? viewer.roles.includes("oem_finance") : false;
}

export function canViewChannelHref(href: string, viewer: Viewer): boolean {
  if (shouldBypassRbac(viewer) || viewer.roles.includes(P)) return true;
  if (!hasAnyRole(viewer.roles, OEM_CONSOLE_ROLES)) return false;
  const path = href.split(/[?#]/)[0];
  if (path === "/channel/delivery") return viewer.channelType === "C";
  if (path === "/channel/staff" || path.startsWith("/channel/staff/")) return viewer.channelType === "C" && viewer.roles.includes("channel_admin");
  if (viewer.channelType === "B" && ["/channel/payments", "/channel/ledger", "/channel/reconciliation"].some(prefix => path === prefix || path.startsWith(`${prefix}/`))) return false;
  if (path.startsWith("/channel/payments/rules")) return viewer.channelType === "C" && hasAnyRole(viewer.roles, ["channel_admin", "oem_finance"]);
  if (viewer.roles.includes("channel_admin")) {
    if (path.startsWith("/channel/subchannels")) return viewer.channelType === "C";
    if (["/channel/plans", "/channel/brand", "/channel/rules", "/channel/margin", "/channel/commission", "/channel/metrics", "/channel/media", "/channel/alerts", "/channel/runbooks", "/channel/audit", "/channel/settings"].some((prefix) => path === prefix || path.startsWith(`${prefix}/`))) return viewer.channelType === "C";
    return true;
  }
  if (viewer.channelType !== "C") return false;
  const common = ["/channel", "/channel/delivery", "/channel/ledger", "/channel/usage", "/channel/margin", "/channel/metrics", "/channel/reconciliation", "/channel/commission", "/channel/commissions", "/channel/settlements", "/channel/alerts", "/channel/runbooks", "/channel/settings", "/channel/rules", "/channel/subchannels", "/channel/payments", "/channel/models"];
  const operational = ["/channel/users", "/channel/keys", "/channel/models", "/channel/plans", "/channel/promos", "/channel/attribution", "/channel/media", "/channel/brand"];
  const allowed = [...common, ...(hasAnyRole(viewer.roles, ["oem_ops", "oem_audit"]) ? operational : []), ...(viewer.roles.includes("oem_audit") ? ["/channel/audit"] : [])];
  return allowed.some((prefix) => prefix === "/channel" ? path === prefix : path === prefix || path.startsWith(`${prefix}/`));
}

export function filterChannelGroups<T extends NavGroupLike>(groups: T[], viewer: Viewer): T[] {
  return groups.map((group) => ({ ...group, items: group.items.filter((item) => canViewChannelHref(item.href, viewer)) })).filter((group) => group.items.length > 0) as T[];
}

export function canAccessPartnerPortal(viewer: Viewer): boolean {
  if (shouldBypassRbac(viewer)) {
    return true;
  }
  return Boolean(viewer.isPartner);
}

export function canAccessUserPortal(viewer: Viewer): boolean {
  return shouldBypassRbac(viewer) || viewer.signedIn;
}

function adminPageKey(href: string): string | undefined {
  const path = href.split(/[?#]/)[0].replace(/\/$/, "") || "/admin";
  if (path === "/admin") {
    return "/admin";
  }
  const keys = Object.keys(ADMIN_PAGE_VIEW)
    .filter((key) => key !== "/admin")
    .sort((a, b) => b.length - a.length);
  return keys.find((key) => path === key || path.startsWith(`${key}/`));
}

export function canViewAdminHref(href: string, viewer: Viewer): boolean {
  if (shouldBypassRbac(viewer)) {
    return true;
  }
  const key = adminPageKey(href);
  if (!key) {
    return hasAnyRole(viewer.roles, [P]);
  }
  return hasAnyRole(viewer.roles, ADMIN_PAGE_VIEW[key] ?? [P]);
}

export function canWrite(action: WriteAction, viewer: Viewer): boolean {
  if (shouldBypassRbac(viewer)) {
    return true;
  }
  return hasAnyRole(viewer.roles, WRITE_ACTION_ROLES[action]);
}

export function filterAdminGroups<T extends NavGroupLike>(groups: T[], viewer: Viewer): T[] {
  if (shouldBypassRbac(viewer)) {
    return groups;
  }
  return groups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => canViewAdminHref(item.href, viewer)),
    }))
    .filter((group) => group.items.length > 0) as T[];
}

export function filterPortalHrefs(hrefs: string[], viewer: Viewer): string[] {
  if (shouldBypassRbac(viewer)) {
    return hrefs;
  }
  return hrefs.filter((href) => {
    if (href === "/admin") {
      return canAccessAdminConsole(viewer.roles);
    }
    if (href === "/channel") {
      return canAccessChannelPortal(viewer.roles);
    }
    if (href === "/partner") {
      return canAccessPartnerPortal(viewer);
    }
    return true;
  });
}
