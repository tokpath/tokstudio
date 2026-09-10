import { ADMIN_CONSOLE_ROLES } from "@/lib/console-home";

/** 顶栏余额只读 GET /v1/me/balance 的这一项：可用 USD 字符串（available_minor 的展示）。不求和、不读 reserved/gift。 */
export const BALANCE_PILL_FIELD = "available" as const;

export const MISSING_PROFILE = "—";
export const EMPTY_KEYS_TITLE = "暂无 API 密钥";

export type BalanceLoadState = "loading" | "ok" | "error";

export type MeProfile = {
  email?: string;
  display_name?: string;
  roles?: string[];
  login_methods?: string[];
};

export type BalanceBody = {
  balance?: {
    available?: string | number;
    available_minor?: number;
    reserved?: string;
    gift_minor?: number;
    commission_available_minor?: number;
  };
};

export function profileDash(value?: string | null): string {
  const text = value?.trim() ?? "";
  return text === "" ? MISSING_PROFILE : text;
}

export function avatarInitial(displayName?: string | null, email?: string | null): string {
  const name = displayName?.trim();
  if (name) {
    return [...name][0]!.toUpperCase();
  }
  const mail = email?.trim();
  if (mail) {
    return mail[0]!.toUpperCase();
  }
  return MISSING_PROFILE;
}

export function shellRole(roles?: string[] | null): "admin" | "user" {
  if (roles?.some((role) => (ADMIN_CONSOLE_ROLES as readonly string[]).includes(role))) {
    return "admin";
  }
  return "user";
}

/** 头像菜单「平台管理」只给 platform_admin，不用 ADMIN_CONSOLE_ROLES。 */
export function canSeePlatformAdmin(roles?: string[] | null): boolean {
  return Boolean(roles?.includes("platform_admin"));
}

/** 只格式化钉死的 available。缺字段或非数字 → 「—」。真实 0 才是 $0.00。 */
export function formatAvailableBalance(available: unknown): string {
  if (available == null || available === "") {
    return MISSING_PROFILE;
  }
  const n = typeof available === "number" ? available : Number(available);
  if (!Number.isFinite(n)) {
    return MISSING_PROFILE;
  }
  return `$${n.toFixed(2)}`;
}

export function balancePillText(state: BalanceLoadState, available: unknown): string {
  if (state === "loading") {
    return "";
  }
  if (state === "error") {
    return MISSING_PROFILE;
  }
  return formatAvailableBalance(available);
}

export function readNailedAvailable(body: BalanceBody | null | undefined): unknown {
  return body?.balance?.[BALANCE_PILL_FIELD];
}

export function loginMethodsOf(methods?: string[] | null): string[] {
  if (!methods?.length) {
    return [];
  }
  return methods.filter((item) => item === "password" || item === "google");
}
