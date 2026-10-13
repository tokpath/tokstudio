import { apiBase } from "@/lib/api";
import { loginHref, safeNextPath } from "@/lib/login-next";
import { resolveStartUsingHref as resolveModelStartHref } from "@/lib/model-use";
import type { CatalogModel } from "@/lib/catalog";

/** 公共站「控制台」入口：未登录去登录，登录后按角色落到对应台。 */
export const CONSOLE_ENTRY_PATH = "/enter";

export function playgroundHref(modelId?: string, from?: string): string {
  const params = new URLSearchParams();
  const id = modelId?.trim();
  if (id) {
    params.set("model", id);
  }
  const catalog = safeNextPath(from);
  if (catalog) {
    params.set("from", catalog);
  }
  const qs = params.toString();
  return qs ? `/app/playground?${qs}` : "/app/playground";
}

export const ADMIN_CONSOLE_ROLES = [
  "platform_admin",
  "finance_admin",
  "ops_admin",
  "tech_admin",
  "audit_readonly",
] as const;

export const OEM_CONSOLE_ROLES = ["channel_admin", "oem_ops", "oem_finance", "oem_audit"] as const;

type MeBody = { user?: { id?: string; roles?: string[] } };

const workspaceKey = (userID: string) => `console-workspace:${userID}`;
const inWorkspace = (path: string, root: string) => path === root || path.startsWith(`${root}/`);
export function rememberConsoleWorkspace(userID: string | undefined, path: string, roles: string[]) {
  if (!userID || typeof window === "undefined") return;
  const root = ["/admin", "/channel", "/app"].find(root => inWorkspace(path, root));
  if (!root) return;
  if (root === "/admin" && !roles.some(role => (ADMIN_CONSOLE_ROLES as readonly string[]).includes(role))) return;
  if (root === "/channel" && !roles.some(role => (OEM_CONSOLE_ROLES as readonly string[]).includes(role))) return;
  // Personal settings and API tools must not replace an employee's management home.
  if (root === "/app" && consoleHomeForRoles(roles) !== "/app") return;
  try { window.localStorage.setItem(workspaceKey(userID), root); } catch { /* Private browsing can disallow storage. */ }
}

export function consoleHomeForRoles(roles: string[] | undefined | null): string {
  const list = roles ?? [];
  if (list.some((role) => (ADMIN_CONSOLE_ROLES as readonly string[]).includes(role))) {
    return "/admin";
  }
  if (list.some((role) => (OEM_CONSOLE_ROLES as readonly string[]).includes(role))) {
    return "/channel";
  }
  return "/app";
}

export function consoleHomeForViewer(input: {
  roles?: string[] | null;
  isPartner?: boolean;
  partnerRole?: string;
}): string {
  const home = consoleHomeForRoles(input.roles);
  if (home !== "/app") {
    return home;
  }
  // Every registered user has a personal promotion role; keep their API workspace as home.
  return "/app";
}

export async function resolveConsoleHref(fetcher: typeof fetch = fetch, next?: string | null): Promise<string> {
  try {
    const meRes = await fetcher(`${apiBase}/v1/me`, { credentials: "include" });
    if (!meRes.ok) {
      return loginHref(CONSOLE_ENTRY_PATH);
    }
    const body = (await meRes.json()) as MeBody;
    const roles = body.user?.roles ?? [];
    let remembered: string | null = null;
    if (body.user?.id && typeof window !== "undefined") {
      try { remembered = window.localStorage.getItem(workspaceKey(body.user.id)); } catch { /* Storage is optional. */ }
    }
    const { canViewAdminHref, canViewChannelHref, canViewUserHref } = await import("./rbac");
    const viewer = { signedIn: true, loading: false, roles, userId: body.user?.id, channelType: undefined as string | undefined };
    const home = consoleHomeForRoles(roles);
    const explicit = safeNextPath(next);
    for (const [index, candidate] of [explicit, safeNextPath(remembered)].entries()) {
      if (!candidate) continue;
      const canonical = candidate.replace(/^\/console(?=\/|[?#]|$)/, "/app");
      const path = canonical.split(/[?#]/)[0];
      if (inWorkspace(path, "/admin") && canViewAdminHref(canonical, viewer)) return canonical;
      if (inWorkspace(path, "/channel") && roles.some(role => (OEM_CONSOLE_ROLES as readonly string[]).includes(role))) {
        const response = await fetcher(`${apiBase}/channel/me`, { credentials: "include" });
        if (response.ok) viewer.channelType = (await response.json()).channel_type;
        if (viewer.channelType && canViewChannelHref(canonical, viewer)) return canonical;
      }
      if (inWorkspace(path, "/app") && (home === "/app" || (index === 0 && path !== "/app")) && canViewUserHref(canonical, viewer)) return canonical;
      // Keep public task return URLs, but never redirect back into an auth loop or a protected portal.
      if (index === 0 && !["/admin", "/channel", "/app", "/partner", "/login", "/enter", "/verify-email", "/forgot-password", "/reset-password"].some(root => inWorkspace(path, root))) return canonical;
    }
    return home;
  } catch {
    return loginHref(CONSOLE_ENTRY_PATH);
  }
}

export async function resolveStartUsingHref(
  model: (Pick<CatalogModel, "id"> & Partial<CatalogModel>) | string,
  fetcher: typeof fetch = fetch,
): Promise<string> {
  const item = typeof model === "string" ? { id: model } : model;
  return resolveModelStartHref(item, fetcher);
}
