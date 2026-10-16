"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { type ReactNode } from "react";
import { useProduct } from "./context";
import { customerNav, operatorNav } from "@/lib/navigation";
import { Empty, ErrorPanel, Loading, Notice } from "./shared";

export function WorkspaceShell({ children, operatorID }: { children: ReactNode; operatorID?: string }) {
  const { session, sessionError, reload } = useProduct(), path = usePathname();
  if (sessionError) return <main className="mx-auto max-w-3xl p-8"><ErrorPanel error={sessionError} retry={reload} /></main>;
  if (!session) return <div className="mx-auto max-w-3xl px-5 py-16"><Loading /></div>;
  if (session.kind === "anonymous") return <main className="mx-auto max-w-xl px-5 py-20"><Empty title="登录后查看你的工作空间"><Link href="/login" className="text-brand-emphasis underline">前往登录</Link></Empty></main>;
  if ((!operatorID && session.kind !== "customer") || (operatorID && session.kind !== "staff")) return <main className="mx-auto max-w-2xl p-8"><Notice>当前登录身份无法访问此空间。<Link href="/login" className="ml-2 underline">使用对应身份登录</Link></Notice></main>;
  const workspace = session.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  if (operatorID && !workspace) return <main className="mx-auto max-w-2xl p-8"><Empty title="没有这个经营空间的访问权限">请联系经营方负责人授予相应人员权限。</Empty></main>;
  const linkClass = (href: string) => `block rounded-control px-3 py-2.5 text-sm transition-colors ${path === href ? "bg-brand-soft font-medium text-brand-emphasis" : "text-ink-secondary hover:bg-canvas-raised"}`;
  const navigation = <>
    {workspace ? <>{operatorNav.filter(group => group.group !== "平台技术" || workspace.operator.kind === "platform").map(group => { const items = group.items.filter(([resource]) => workspace.permissions.includes("*") || workspace.permissions.includes(`${resource}.read`)); return items.length ? <div key={group.group} className="mb-5"><p className="mb-2 px-3 text-xs text-ink-mute">{group.group}</p>{items.map(([resource, label]) => { const href = `/manage/${operatorID}/${resource}`; return <Link key={resource} href={href} className={linkClass(href)} aria-current={path === href ? "page" : undefined}>{label}</Link>; })}</div> : null; })}</> : customerNav.map(([resource, label]) => { const href = `/workspace${resource ? `/${resource}` : ""}`; return <Link key={resource} href={href} className={linkClass(href)} aria-current={path === href ? "page" : undefined}>{label}</Link>; })}
  </>;
  return <div className="mx-auto grid max-w-[1500px] lg:grid-cols-[240px_minmax(0,1fr)]"><aside className="border-b border-hairline p-4 lg:min-h-[calc(100vh-200px)] lg:border-b-0 lg:border-r lg:p-5"><p className="mb-4 px-3 text-sm font-semibold">{workspace?.operator.name || (session.kind === "customer" ? session.operator.name : "工作空间")}</p>{session.kind === "staff" && session.workspaces.length > 1 && <select aria-label="经营空间" className="mb-5 w-full rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm" value={operatorID} onChange={event => { window.location.assign(`/manage/${encodeURIComponent(event.target.value)}/overview`); }}>{session.workspaces.map(item => <option value={item.operator.id} key={item.operator.id}>{item.operator.name}</option>)}</select>}<nav aria-label="工作台" className="hidden lg:block">{navigation}</nav><details className="lg:hidden"><summary className="cursor-pointer px-3 py-2 text-sm text-brand-emphasis">打开工作台导航</summary><nav className="mt-3 grid grid-cols-2" aria-label="移动工作台">{navigation}</nav></details></aside><main className="min-w-0 px-5 py-8 lg:px-9 lg:py-10">{children}</main></div>;
}
