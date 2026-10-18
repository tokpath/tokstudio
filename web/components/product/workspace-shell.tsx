"use client";
import { useI18n } from "@/components/i18n";
import Link from "next/link";
import { allowNavigation } from "@/lib/navigation-guard";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { Bell, BookOpen, ChartNoAxesCombined, ChevronRight, CircleUserRound, CreditCard, Gift, Handshake, KeyRound, LayoutDashboard, Menu, Package, Settings2, SlidersHorizontal, TerminalSquare, Users, X } from "lucide-react";
import { useProduct } from "./context";
import { useQuery } from "@tanstack/react-query";
import { CreditsSchema } from "@/lib/contracts";
import { authContinuation, safeCustomerNext } from "@/lib/customer-next";
import { request } from "@/lib/api-client";
import { customerSections, operatorLanding, sectionForResource, visibleOperatorSections } from "@/lib/navigation";
import { Empty, ErrorPanel, Loading, Notice } from "./shared";

const icons = { overview: LayoutDashboard, models: BookOpen, keys: KeyRound, usage: ChartNoAxesCombined, funds: CreditCard, promotion: Gift, account: CircleUserRound, customers: Users, products: Package, marketing: Gift, finance: CreditCard, cooperation: Handshake, reports: ChartNoAxesCombined, settings: Settings2, technology: TerminalSquare };
const kindNames: Record<string, string> = { platform: "平台经营空间", oem: "OEM 经营空间", channel: "渠道经营空间" };
export function WorkspaceShell({ children, operatorID }: { children: ReactNode; operatorID?: string }) {
  const { t } = useI18n();
  const { session, sessionError, reload } = useProduct();
  const path = usePathname(), router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false), [continuation, setContinuation] = useState<string>(), [collapsedSection, setCollapsedSection] = useState<string>();
  const credits = useQuery({ queryKey: ["credits"], queryFn: () => request("/api/v1/me/credits", CreditsSchema), enabled: session?.kind === "customer" });
  useEffect(() => { setMobileOpen(false); setCollapsedSection(undefined); setContinuation(safeCustomerNext(path + window.location.search)); }, [path]);
  if (sessionError) return <main className="mx-auto max-w-3xl p-8"><ErrorPanel error={sessionError} retry={reload} /></main>;
  if (!session) return <main className="mx-auto max-w-3xl px-5 py-16"><Loading /></main>;
  if (session.kind === "anonymous") return <main className="mx-auto max-w-xl px-5 py-20"><Empty title={t("登录后查看你的工作空间")}><Link href={authContinuation("/login", continuation)} className="text-brand-emphasis underline">{t("前往登录")}</Link></Empty></main>;
  if (session.kind === "staff" && session.requires_password_change) return <main className="mx-auto max-w-xl px-5 py-20"><Notice>{t("请先修改初始密码，再进入经营空间")}<Link href="/change-password" className="ml-2 underline">{t("修改初始密码")}</Link></Notice></main>;
  if ((!operatorID && session.kind !== "customer") || (operatorID && session.kind !== "staff")) return <main className="mx-auto max-w-2xl p-8"><Notice>{t("当前登录身份无法访问此空间。")}<Link href="/login" className="ml-2 underline">{t("使用对应身份登录")}</Link></Notice></main>;
  const workspace = session.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  if (operatorID && !workspace) return <main className="mx-auto max-w-2xl p-8"><Empty title={t("没有这个经营空间的访问权限")}>{t("请联系经营方负责人授予相应人员权限。")}</Empty></main>;
  const sections = workspace ? visibleOperatorSections(workspace) : customerSections.map(section => section.id === "promotion" && !credits.data?.qualification.qualified_at ? { ...section, items: section.items.filter(([item]) => item !== "activities") } : section);
  const base = operatorID ? `/manage/${encodeURIComponent(operatorID)}` : "/workspace";
  const resource = path.slice(base.length).replace(/^\//, "");
  const activeSection = sectionForResource(sections, resource);
  const hrefFor = (resource: string) => `${base}${resource ? `/${resource}` : ""}`;
  const displayName = workspace?.operator.name || (session.kind === "customer" ? session.operator.name : t("工作空间"));
  const notificationHref = workspace ? (workspace.permissions.includes("*") || workspace.permissions.includes("notifications.read") ? `${base}/notifications` : undefined) : "/workspace/notifications";
  const primarySections = sections.filter(section => section.id !== "account").map(section => ({ ...section, items: section.items.filter(([item]) => item !== "notifications") })).filter(section => section.items.length);
  const tools = workspace ? notificationHref ? [["notifications", "通知管理"]] : [] : customerSections.find(section => section.id === "account")!.items;
  const utilityLinks = tools.map(([item, label]) => { const Icon = item === "notifications" ? Bell : CircleUserRound; return <Link key={item} href={hrefFor(item)} aria-current={resource.split("/")[0] === item ? "page" : undefined} className={`flex min-h-11 items-center gap-3 rounded-control px-3 py-2.5 text-sm ${resource.split("/")[0] === item ? "bg-brand-soft font-semibold text-brand-emphasis" : "text-ink-secondary hover:bg-canvas"}`}><Icon size={17} aria-hidden="true" />{t(label)}</Link>; });
  const navigation = primarySections.map(section => {
    const Icon = icons[section.id as keyof typeof icons] || SlidersHorizontal;
    const active = section.id === activeSection?.id;
    const expandable = section.items.length > 1;
    const expanded = expandable && active && collapsedSection !== section.id;
    return <div key={section.id}><Link href={hrefFor(section.items[0][0])} aria-current={active && !expandable ? "page" : undefined} aria-expanded={expandable ? expanded : undefined} onClick={event => { if (!expandable || !active) return; if (!allowNavigation()) { event.preventDefault(); return; } event.preventDefault(); setCollapsedSection(expanded ? section.id : undefined); }} className={`flex min-h-11 items-center gap-3 rounded-control px-3 py-3 text-sm transition-colors ${active ? "bg-brand-soft font-semibold text-brand-emphasis" : "text-ink-secondary hover:bg-canvas hover:text-ink"}`}><Icon size={18} strokeWidth={1.7} aria-hidden="true" /><span>{t(section.group)}</span>{expandable && <ChevronRight size={14} className={`ml-auto transition-transform ${expanded ? "rotate-90" : ""}`} aria-hidden="true" />}</Link>{expanded && <nav aria-label={t("{value0}内导航", { value0: t(section.group) })} className="mb-2 ml-5 mt-1 space-y-0.5 border-l border-hairline pl-3">{section.items.map(([item, label]) => <Link key={item} href={hrefFor(item)} aria-current={resource.split("/")[0] === item ? "page" : undefined} className={`block min-h-10 rounded-control px-3 py-2.5 text-sm ${resource.split("/")[0] === item ? "font-semibold text-brand-emphasis" : "text-ink-secondary hover:bg-canvas"}`}>{t(label)}</Link>)}</nav>}</div>;
  });
  return <div className="mx-auto grid max-w-[1600px] lg:grid-cols-[220px_minmax(0,1fr)]">
    <aside className="border-b border-hairline bg-canvas-raised px-4 py-4 lg:sticky lg:top-0 lg:flex lg:max-h-screen lg:min-h-[calc(100vh-70px)] lg:flex-col lg:self-start lg:border-b-0 lg:border-r lg:px-4 lg:py-6">
      <div className="flex items-center justify-between gap-3 lg:block"><div className="min-w-0 px-2"><p className="mb-1 text-[11px] font-medium text-ink-mute">{workspace ? t(kindNames[workspace.operator.kind]) : t("客户工作台")}</p><p className="truncate text-sm font-semibold" title={displayName}>{displayName}</p></div><button type="button" aria-label={mobileOpen ? t("收起工作台导航") : t("打开工作台导航")} aria-expanded={mobileOpen} aria-controls="mobile-workspace-nav" onClick={() => setMobileOpen(value => !value)} className="flex min-h-11 items-center gap-2 rounded-control px-3 text-sm lg:hidden">{mobileOpen ? <X size={18} /> : <Menu size={18} />}<span>{activeSection ? t(activeSection.group) : t("导航")}</span></button></div>
      {session.kind === "staff" && session.workspaces.length > 1 && <label className="mt-4 block px-2 text-xs text-ink-mute">{t("切换已授权经营空间")}<select aria-label={t("经营空间")} className="mt-2 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-2 text-sm text-ink" value={operatorID} onChange={event => { if (!allowNavigation()) return; const target = session.workspaces.find(item => item.operator.id === event.target.value); if (target) router.push(operatorLanding(target)); }}>{session.workspaces.map(item => <option value={item.operator.id} key={item.operator.id}>{item.operator.name}</option>)}</select></label>}
      <nav aria-label={t("工作台")} className="sidebar-scroll mt-6 hidden min-h-0 space-y-1 lg:block">{navigation}</nav>
      <div id="mobile-workspace-nav" className={`${mobileOpen ? "block" : "hidden"} mt-4 lg:hidden`}><nav aria-label={t("移动工作台")} className="space-y-1">{navigation}</nav><nav aria-label={t("移动工作台工具")} className="mt-4 border-t border-hairline pt-3">{utilityLinks}</nav></div>
      <nav aria-label={t("工作台工具")} className="mt-auto hidden shrink-0 space-y-1 border-t border-hairline pt-4 lg:block">{utilityLinks}</nav><div className="mt-4 hidden border-t border-hairline px-2 pt-4 text-xs leading-6 text-ink-mute lg:block">{workspace ? t("仅显示当前经营空间已授权的业务。") : t("套餐、额度和 Key 均由当前经营方提供。")}<Link className="mt-2 block text-brand-emphasis hover:underline" href={workspace ? "/" : "/workspace/models"}>{workspace ? t("查看品牌网站") : t("查看接入指南")} <span aria-hidden="true">↗</span></Link></div>
    </aside>
    <div className="min-w-0">
      <div className="flex min-h-14 items-center justify-between gap-3 border-b border-hairline px-5 lg:px-8"><p className="flex items-center gap-2 text-xs text-ink-mute"><span>{workspace ? t("经营管理") : t("我的服务")}</span><ChevronRight size={12} aria-hidden="true" /><span className="font-medium text-ink-secondary">{activeSection ? t(activeSection.group) : t("业务详情")}</span></p></div>
      <main id="workspace-content" className="min-w-0 px-5 py-7 lg:px-8 lg:py-8" key={`${base}/${resource.split("/")[0]}`}>{children}</main>
    </div>
  </div>;
}
