"use client";
import { useI18n, LanguageControl } from "@/components/i18n";
import Link from "next/link";
import { allowNavigation } from "@/lib/navigation-guard";
import { createContext, useContext, type ReactNode, type CSSProperties } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { ThemeSchema, PublicContextSchema, SessionSchema, CommandResultSchema, type PublicContext, type Session } from "@/lib/contracts";
import { request, setCSRFToken } from "@/lib/api-client";
import { safeAssetURL } from "@/lib/decimal";
import { resolveTheme } from "@/lib/theme";
import { operatorLanding } from "@/lib/navigation";
import { Monitor } from "lucide-react";
import { StaffNotificationBell } from "./staff-notification-bell";
import { useTheme } from "next-themes";
import { Button } from "@/components/ui/button";
import { ErrorPanel, useCommand } from "./shared";

const Context = createContext<{ context?: PublicContext; session?: Session; sessionError?: unknown; reload: () => void }>({ reload: () => {} });
export function useProduct() { return useContext(Context); }
export function ProductProvider({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const pathname = usePathname();
  const insideWorkspace = pathname.startsWith("/workspace") || pathname.startsWith("/manage/");
  const context = useQuery({ queryKey: ["public-context"], queryFn: () => request("/api/v1/public/context", PublicContextSchema) });
  const session = useQuery({ queryKey: ["session"], queryFn: async () => { const value = await request("/api/v1/auth/me", SessionSchema); setCSRFToken(value.kind !== "anonymous" ? value.csrf_token : undefined); return value; } });
  const current = session.data;
  const presentationOperator = current?.kind === "customer" ? current.operator : context.data?.operator;
  const theme = ThemeSchema.parse(resolveTheme(resolveTheme(context.data?.brand.theme, presentationOperator?.theme), current?.kind === "customer" ? current.operator.theme : context.data?.theme));
  const style = { ...(theme.primary_color ? { "--oem-brand": theme.primary_color, "--brand": theme.primary_color, "--brand-primary": theme.primary_color, "--brand-soft": `color-mix(in srgb, ${theme.primary_color} 12%, var(--canvas-raised))` } : {}), ...(theme.accent_color ? { "--oem-brand-press": theme.accent_color, "--brand-press": theme.accent_color } : {}), ...(theme.background_color ? { "--branding-background": theme.background_color } : {}), ...(theme.text_color ? { "--branding-text": theme.text_color } : {}) } as CSSProperties;
  const rawLogo = safeAssetURL(theme.logo_url);const logo=rawLogo?.startsWith("/v1/branding/assets/")?`/api${rawLogo}?operator_id=${encodeURIComponent(presentationOperator?.id||"")}`:rawLogo;
 const activeStaffWorkspace = current?.kind === "staff" ? current.workspaces.find(item => pathname.startsWith(`/manage/${item.operator.id}/`)) : undefined;
 const actualOperator = current?.kind === "customer" ? current.operator : activeStaffWorkspace?.operator || context.data?.operator;

  return <Context.Provider value={{ context: context.data, session: current, sessionError: session.error, reload: () => { void context.refetch(); void session.refetch(); } }}>
    <div style={style} className="product-theme min-h-screen bg-canvas text-ink">
      <a href={insideWorkspace ? "#workspace-content" : "#site-content"} className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-control focus:bg-canvas-raised focus:px-4 focus:py-3">{t("跳到页面内容")}</a>
      <header className="border-b border-hairline bg-canvas-raised">
        <div className="mx-auto flex max-w-[1500px] flex-wrap items-center gap-6 px-5 py-4 lg:px-8">
          <Link href="/" className="flex shrink-0 items-center gap-3 whitespace-nowrap font-semibold tracking-tight">{logo ? <img alt="" src={logo} className="h-8 w-8 object-contain" /> : <span className="grid h-8 w-8 place-items-center rounded-stamp bg-brand text-on-brand">T</span>}<span>{context.data?.brand.name || "TokenHub"}</span></Link>
          <nav aria-label={t("网站")} className={`${insideWorkspace ? "hidden md:flex" : "order-3 flex w-full sm:order-none sm:w-auto"} items-center gap-5 whitespace-nowrap text-sm text-ink-secondary sm:flex-1`}><Link href="/models" aria-current={pathname === "/models" ? "page" : undefined}>{t("模型与价格")}</Link><Link href="/plans" aria-current={pathname === "/plans" ? "page" : undefined}>{t("通用套餐")}</Link></nav>
          {current?.kind === "customer" ? <div className="ml-auto flex shrink-0 items-center gap-3 text-sm"><Link href="/workspace">{t("客户工作台")}</Link><span className="hidden text-ink-mute md:inline">{current.customer.display_name}</span><Logout /></div> : current?.kind === "staff" ? <div className="ml-auto flex shrink-0 items-center gap-3 text-sm"><Link href={current.workspaces.length ? operatorLanding(current.workspaces.find(item => item.operator.id === current.active_operator_id) || current.workspaces[0]) : "/login"}>{t("经营管理")}</Link><StaffNotificationBell workspaces={current.workspaces} activeOperatorID={current.active_operator_id} pathname={pathname} /><span className="hidden text-ink-mute md:inline">{current.staff.display_name}</span><Logout /></div> : <div className="ml-auto flex shrink-0 items-center gap-4 text-sm"><Link href="/login">{t("登录")}</Link><Button asChild size="sm"><Link href="/register">{t("创建账号")}</Link></Button></div>}
          <LanguageControl /><ThemeControl />
        </div>
      </header>
      {context.isError && <div className="mx-auto max-w-[1500px] px-5 pt-4"><ErrorPanel error={context.error} retry={() => void context.refetch()} compact /></div>}
      {context.data && !insideWorkspace && <div className="border-b border-hairline bg-canvas px-5 py-2 text-center text-xs text-ink-secondary">{t("品牌：")}{context.data.brand.name}<span className="mx-3 text-hairline">/</span>{t("实际经营方：")}{current?.kind === "customer" ? current.operator.name : context.data.operator.name}{actualOperator?.legal_name && <span> · {actualOperator.legal_name}</span>}</div>}
      <div id="site-content">{children}</div>
      <footer className="border-t border-hairline px-5 py-8 text-center text-xs leading-6 text-ink-mute">{actualOperator?.name || "TokenHub"} {t("· 模型服务与独立经营")}{actualOperator?.contact_email && <span> · <a href={`mailto:${actualOperator.contact_email}`}>{actualOperator.contact_email}</a></span>}<p>{t("价格和权益以本次成交条款为准。账单、资金和服务责任均可追溯。")}</p></footer>
    </div>
  </Context.Provider>;
}
function Logout() {
  const { t } = useI18n();
  const command = useCommand("/api/v1/auth/logout", CommandResultSchema);
 const queryClient = useQueryClient(), router = useRouter();
  return <span><button className="text-ink-mute underline" disabled={command.busy} onClick={async () => { if (!allowNavigation()) return; const result = await command.run({}); if (result) { setCSRFToken(); queryClient.removeQueries({ predicate: query => !["public-context", "session"].includes(String(query.queryKey[0])) }); queryClient.setQueryData(["session"], { kind: "anonymous" }); router.replace("/login"); } }}>{command.unresolved ? t("重试退出") : t("退出")}</button>{!!command.error && <span role="alert" className="ml-2 text-danger">{command.error instanceof Error ? t(command.error.message) : t("退出失败")}</span>}</span>;
}

function ThemeControl() {
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  return <label className="flex items-center gap-1 text-ink-mute"><Monitor size={15} aria-hidden="true" /><select aria-label={t("显示主题")} value={theme || "system"} onChange={event => setTheme(event.target.value)} className="min-h-10 max-w-40 rounded-control bg-transparent px-1 text-xs"><option value="system">{t("跟随系统")}</option><option value="light">{t("浅色")}</option><option value="dark">{t("深色")}</option></select></label>;
}
