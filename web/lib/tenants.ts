export type TenantListKind = "channel" | "agent" | "kol";

export const CHANNEL_TYPES = [
  { value: "A", label: "平台" },
  { value: "B", label: "渠道" },
  { value: "C", label: "OEM" },
] as const;

export const ROLE_TYPES = [
  { value: "agent", label: "代理商" },
  { value: "promoter", label: "个人推广员" },
] as const;

export const STATUS_OPTIONS = [
  { value: "active", label: "active" },
  { value: "disabled", label: "disabled" },
] as const;

export function channelTypeLabel(type: string): string {
  return CHANNEL_TYPES.find((item) => item.value === type)?.label ?? type;
}

export function roleTypeLabel(type: string): string {
  if (["kol_l1", "kol_l2"].includes(type)) return "个人推广员";
  return ROLE_TYPES.find((item) => item.value === type)?.label ?? type;
}

export function isKOLType(type: string): boolean {
  return type === "promoter" || type === "kol_l1" || type === "kol_l2";
}

export function channelUsesQuota(type: string): boolean {
  return type === "C";
}

export function channelHref(id: string): string {
  return `/admin/channels/${encodeURIComponent(id)}`;
}

export function partnerHref(id: string): string {
  return `/admin/partners/${encodeURIComponent(id)}`;
}

export function canGrantTenantModels(roles: string[] | undefined): boolean {
  return Boolean(roles?.some((role) => role === "platform_admin" || role === "ops_admin"));
}

export function adminNavActive(pathname: string, href: string): boolean {
  if (href === "/admin") {
    return pathname === "/admin";
  }
  if (href === "/admin/channels") {
    return pathname === href || pathname.startsWith("/admin/channels/") || pathname.startsWith("/admin/partners/");
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}
