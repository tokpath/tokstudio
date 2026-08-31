export type TenantListKind = "channel" | "agent" | "kol";

export const CHANNEL_TYPES = [
  { value: "A", label: "A 平台直推" },
  { value: "B", label: "B 批发商" },
  { value: "C", label: "C OEM" },
] as const;

export const ROLE_TYPES = [
  { value: "agent", label: "代理商" },
  { value: "kol_l1", label: "1 级 KOL" },
  { value: "kol_l2", label: "2 级 KOL" },
] as const;

export const STATUS_OPTIONS = [
  { value: "active", label: "active" },
  { value: "disabled", label: "disabled" },
] as const;

export function channelTypeLabel(type: string): string {
  return CHANNEL_TYPES.find((item) => item.value === type)?.label ?? type;
}

export function roleTypeLabel(type: string): string {
  return ROLE_TYPES.find((item) => item.value === type)?.label ?? type;
}

export function isKOLType(type: string): boolean {
  return type === "kol_l1" || type === "kol_l2";
}

export function channelUsesQuota(type: string): boolean {
  return type === "B" || type === "C";
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
