import { apiBase } from "@/lib/api";
import { loginHref } from "@/lib/login-next";

/** 公共站「控制台」入口：未登录去登录，登录后按角色落到对应台。 */
export const CONSOLE_ENTRY_PATH = "/enter";

export const ADMIN_CONSOLE_ROLES = [
  "platform_admin",
  "finance_admin",
  "ops_admin",
  "tech_admin",
  "audit_readonly",
] as const;

type MeBody = { user?: { roles?: string[] } };

export function consoleHomeForRoles(roles: string[] | undefined | null): string {
  const list = roles ?? [];
  if (list.some((role) => (ADMIN_CONSOLE_ROLES as readonly string[]).includes(role))) {
    return "/admin";
  }
  if (list.includes("channel_admin")) {
    return "/channel";
  }
  return "/app";
}

export function consoleHomeForViewer(input: {
  roles?: string[] | null;
  isPartner?: boolean;
}): string {
  const home = consoleHomeForRoles(input.roles);
  if (home !== "/app") {
    return home;
  }
  return input.isPartner ? "/partner" : "/app";
}

export async function resolveConsoleHref(fetcher: typeof fetch = fetch): Promise<string> {
  try {
    const meRes = await fetcher(`${apiBase}/v1/me`, { credentials: "include" });
    if (!meRes.ok) {
      return loginHref(CONSOLE_ENTRY_PATH);
    }
    const body = (await meRes.json()) as MeBody;
    const roles = body.user?.roles;
    const rbacHome = consoleHomeForRoles(roles);
    if (rbacHome !== "/app") {
      return rbacHome;
    }
    try {
      const partnerRes = await fetcher(`${apiBase}/v1/partner/me`, { credentials: "include" });
      return consoleHomeForViewer({ roles, isPartner: partnerRes.ok });
    } catch {
      return "/app";
    }
  } catch {
    return loginHref(CONSOLE_ENTRY_PATH);
  }
}
