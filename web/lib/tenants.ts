import { defaultCopy, type CopyTranslator } from "@/lib/copy";
export type TenantListKind = "channel" | "agent" | "kol";

export function channelTypes(tUI: CopyTranslator = defaultCopy) { return [
  { value: "A", label: tUI("common.ownerPlatform") },
  { value: "B", label: tUI("chrome.groupChannel") },
  { value: "C", label: "OEM" },
] as const; }
export const CHANNEL_TYPES = channelTypes();

export function roleTypes(tUI: CopyTranslator = defaultCopy) { return [
  { value: "agent", label: tUI("partnerBoard.roleAgent") },
  { value: "promoter", label: tUI("partnerBoard.rolePromoter") },
] as const; }
export const ROLE_TYPES = roleTypes();

export const STATUS_OPTIONS = [
  { value: "active", label: "active" },
  { value: "disabled", label: "disabled" },
] as const;

export function channelTypeLabel(type: string, tUI: CopyTranslator = defaultCopy): string {
  return channelTypes(tUI).find((item) => item.value === type)?.label ?? type;
}

export function roleTypeLabel(type: string, tUI: CopyTranslator = defaultCopy): string {
  if (["kol_l1", "kol_l2"].includes(type)) return tUI("partnerBoard.rolePromoter");
  return roleTypes(tUI).find((item) => item.value === type)?.label ?? type;
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
    return pathname === href || pathname.startsWith("/admin/channels/") || pathname.startsWith("/admin/partners/") || pathname.startsWith("/admin/oem-deliveries/");
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}
