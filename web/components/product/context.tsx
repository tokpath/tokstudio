"use client";
import Link from "next/link";
import { createContext, useContext, type ReactNode, type CSSProperties } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { ThemeSchema, PublicContextSchema, SessionSchema, CommandResultSchema, type PublicContext, type Session } from "@/lib/contracts";
import { request, setCSRFToken } from "@/lib/api-client";
import { safeAssetURL } from "@/lib/decimal";
import { resolveTheme } from "@/lib/theme";
import { Button } from "@/components/ui/button";
import { ErrorPanel, useCommand } from "./shared";

const Context = createContext<{ context?: PublicContext; session?: Session; sessionError?: unknown; reload: () => void }>({ reload: () => {} });
export function useProduct() { return useContext(Context); }
export function ProductProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const context = useQuery({ queryKey: ["public-context"], queryFn: () => request("/api/v1/public/context", PublicContextSchema) });
  const session = useQuery({ queryKey: ["session"], queryFn: async () => { const value = await request("/api/v1/auth/me", SessionSchema); setCSRFToken(value.kind !== "anonymous" ? value.csrf_token : undefined); return value; } });
  const current = session.data;
  const presentationOperator = current?.kind === "customer" ? current.operator : context.data?.operator;
  const theme = ThemeSchema.parse(resolveTheme(resolveTheme(context.data?.brand.theme, presentationOperator?.theme), current?.kind === "customer" ? current.operator.theme : context.data?.theme));
  const style = { ...(theme.primary_color ? { "--oem-brand": theme.primary_color, "--brand": theme.primary_color, "--brand-primary": theme.primary_color, "--brand-emphasis": theme.primary_color, "--brand-soft": `color-mix(in srgb, ${theme.primary_color} 12%, var(--canvas-raised))` } : {}), ...(theme.accent_color ? { "--oem-brand-press": theme.accent_color, "--brand-press": theme.accent_color } : {}), ...(theme.background_color ? { "--canvas": theme.background_color, "--canvas-raised": theme.background_color } : {}), ...(theme.text_color ? { "--ink": theme.text_color } : {}) } as CSSProperties;
  const rawLogo = safeAssetURL(theme.logo_url);const logo=rawLogo?.startsWith("/v1/branding/assets/")?`/api${rawLogo}?operator_id=${encodeURIComponent(presentationOperator?.id||"")}`:rawLogo;
 const actualOperator = current?.kind === "customer" ? current.operator : context.data?.operator;

  return <Context.Provider value={{ context: context.data, session: current, sessionError: session.error, reload: () => { void context.refetch(); void session.refetch(); } }}>
    <div style={style} className="min-h-screen">
      <header className="border-b border-hairline bg-canvas-raised">
        <div className="mx-auto flex max-w-[1500px] flex-wrap items-center gap-6 px-5 py-4 lg:px-8">
          <Link href="/" className="flex shrink-0 items-center gap-3 whitespace-nowrap font-semibold tracking-tight">{logo ? <img alt="" src={logo} className="h-8 w-8 object-contain" /> : <span className="grid h-8 w-8 place-items-center rounded-stamp bg-brand text-on-brand">T</span>}<span>{context.data?.brand.name || "TokenHub"}</span></Link>
          <nav aria-label="网站" className="order-3 flex w-full items-center gap-5 whitespace-nowrap text-sm text-ink-secondary sm:order-none sm:w-auto sm:flex-1"><Link href="/models" aria-current={pathname === "/models" ? "page" : undefined}>模型与价格</Link><Link href="/plans" aria-current={pathname === "/plans" ? "page" : undefined}>通用套餐</Link></nav>
          {current?.kind === "customer" ? <div className="ml-auto flex shrink-0 items-center gap-3 text-sm"><Link href="/workspace">客户工作台</Link><span className="hidden text-ink-mute md:inline">{current.customer.display_name}</span><Logout /></div> : current?.kind === "staff" ? <div className="ml-auto flex shrink-0 items-center gap-3 text-sm"><Link href={`/manage/${current.active_operator_id || current.workspaces[0]?.operator.id || "unassigned"}/overview`}>经营管理</Link><span className="hidden text-ink-mute md:inline">{current.staff.display_name}</span><Logout /></div> : <div className="ml-auto flex shrink-0 items-center gap-4 text-sm"><Link href="/login">登录</Link><Button asChild size="sm"><Link href="/register">创建账号</Link></Button></div>}
        </div>
      </header>
      {context.isError && <div className="mx-auto max-w-[1500px] px-5 pt-4"><ErrorPanel error={context.error} retry={() => void context.refetch()} compact /></div>}
      {context.data && <div className="border-b border-hairline bg-canvas px-5 py-2 text-center text-xs text-ink-secondary">品牌：{context.data.brand.name}<span className="mx-3 text-hairline">/</span>实际经营方：{current?.kind === "customer" ? current.operator.name : context.data.operator.name}{actualOperator?.legal_name && <span> · {actualOperator.legal_name}</span>}</div>}
      {children}
      <footer className="border-t border-hairline px-5 py-8 text-center text-xs leading-6 text-ink-mute">{actualOperator?.name || "TokenHub"} · 模型服务与独立经营{context.data?.operator.contact_email && <span> · <a href={`mailto:${context.data.operator.contact_email}`}>{context.data.operator.contact_email}</a></span>}<p>价格和权益以本次成交条款为准。账单、资金和服务责任均可追溯。</p></footer>
    </div>
  </Context.Provider>;
}
function Logout() {
  const command = useCommand("/api/v1/auth/logout", CommandResultSchema);
 const queryClient = useQueryClient(), router = useRouter();
  return <span><button className="text-ink-mute underline" disabled={command.busy} onClick={async () => { const result = await command.run({}); if (result) { setCSRFToken(); queryClient.removeQueries({ predicate: query => !["public-context", "session"].includes(String(query.queryKey[0])) }); queryClient.setQueryData(["session"], { kind: "anonymous" }); router.replace("/login"); } }}>{command.unresolved ? "重试退出" : "退出"}</button>{!!command.error && <span role="alert" className="ml-2 text-danger">{command.error instanceof Error ? command.error.message : "退出失败"}</span>}</span>;
}
