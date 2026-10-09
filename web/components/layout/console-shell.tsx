"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  adminGroups,
  channelNavGroupsFor,
  channelNavItemForPath,
  isNavActive,
  navItemForPath,
  partnerNavGroups,
  userNavGroups,
  userSettingsNav,
  type NavItem,
} from "@/lib/nav";
import { adminNavActive } from "@/lib/tenants";
import { ChevronDown, Menu, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ThemeToggle } from "@/components/theme-toggle";
import type { Brand } from "@/lib/brand";
import { BrandLogo } from "@/components/brand-logo";
import { LocaleSwitch } from "@/components/locale-switch";
import { ConsoleOverflowMenu } from "@/components/layout/console-overflow-menu";
import { UserShellRightZone } from "@/components/layout/user-shell-menu";
import { iconForHref } from "@/lib/page-icons";
import { canAccessChannelPortal, canAccessPartnerPortal, filterAdminGroups, filterChannelGroups, shouldBypassRbac, canViewUserHref } from "@/lib/rbac";
import { rememberConsoleWorkspace } from "@/lib/console-home";
import { useViewer } from "@/components/rbac/viewer-context";

const navLinkFocus =
  "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand";

function GroupedNav({
  groups,
  pathname,
  t,
  onNavigate,
  isActive = isNavActive,
  collapsible = false,
}: {
  groups: { titleKey: string; items: NavItem[] }[];
  pathname: string;
  t: (key: string) => string;
  onNavigate?: () => void;
  isActive?: (pathname: string, href: string) => boolean;
  collapsible?: boolean;
}) {
  const activeGroup = groups.find(group => group.items.some(item => isActive(pathname, item.href)))?.titleKey;
  const [expanded, setExpanded] = useState<string | undefined>(activeGroup);
  useEffect(() => { setExpanded(activeGroup); }, [activeGroup]);
  return (
    <div className="flex flex-col gap-0">
      {groups.map((group) => (
        <div key={group.titleKey} className={collapsible ? "mb-2" : "mb-3"}>
          {collapsible ? <button type="button" aria-expanded={expanded === group.titleKey} className={`flex min-h-11 w-full items-center justify-between rounded-control px-3 text-left text-sm font-medium text-ink ${navLinkFocus}`} onClick={() => setExpanded(current => current === group.titleKey ? undefined : group.titleKey)}>{t(group.titleKey)}<ChevronDown aria-hidden className={`size-4 transition-transform ${expanded === group.titleKey ? "rotate-180" : ""}`} /></button> : group.titleKey === "tools" ? <p className="th-eyebrow mb-2 px-3 text-ink-mute">{t(group.titleKey)}</p> : null}
          <ul hidden={collapsible && expanded !== group.titleKey} className={collapsible && expanded !== group.titleKey ? "hidden" : "flex flex-col gap-0.5"}>
            {group.items.map((item) => {
              const active = isActive(pathname, item.href);
              const Icon = iconForHref(item.href);
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    onClick={onNavigate}
                    className={`relative flex min-h-11 w-full items-center gap-2.5 rounded-control px-3 py-2 text-sm no-underline transition-colors duration-150 ${navLinkFocus} ${
                      active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:bg-canvas-raised hover:text-ink"
                    }`}
                  >
                    {active ? <span className="absolute inset-y-2 left-0 hidden w-0.5 bg-brand md:block" /> : null}
                    <Icon className="size-4 shrink-0" strokeWidth={1.75} aria-hidden />
                    {t(item.key)}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </div>
  );
}

function ConsoleNav({
  pathname,
  onNavigate,
}: {
  pathname: string;
  onNavigate?: () => void;
}) {
  const tu = useTranslations("userNav");
  const ta = useTranslations("admin");
  const tch = useTranslations("channelNav");
  const tp = useTranslations("partnerNav");
  const isAdmin = pathname.startsWith("/admin");
  const isUser = pathname.startsWith("/app") || pathname.startsWith("/console");
  const isChannel = pathname.startsWith("/channel");
  const isPartner = pathname.startsWith("/partner");
  const viewer = useViewer();
  const adminNav = filterAdminGroups(adminGroups, viewer);
  const showChannelNav = isChannel && (shouldBypassRbac(viewer) || canAccessChannelPortal(viewer.roles));
  const showPartnerNav = isPartner && (shouldBypassRbac(viewer) || canAccessPartnerPortal(viewer));

  return (
    <>
      {isUser ? <GroupedNav groups={userNavGroups.map(group => ({...group, items: group.items.filter(item => canViewUserHref(item.href, viewer))})).filter(group => group.items.length > 0)} pathname={pathname} t={tu} onNavigate={onNavigate} /> : null}
      {showChannelNav ? <GroupedNav groups={filterChannelGroups(channelNavGroupsFor(viewer.channelType), viewer)} pathname={pathname} collapsible={viewer.channelType === "C"} t={viewer.channelType === "C" ? ta : tch} onNavigate={onNavigate} isActive={(path, href) => channelNavItemForPath(path, viewer.channelType)?.href === href} /> : null}
      {showPartnerNav ? <GroupedNav groups={partnerNavGroups} pathname={pathname} t={tp} onNavigate={onNavigate} /> : null}
      {isAdmin ? (
        <GroupedNav groups={adminNav} collapsible pathname={pathname} t={ta} onNavigate={onNavigate} isActive={adminNavActive} />
      ) : null}
    </>
  );
}

export function ConsoleShell({
  brand,
  children,
  onCommand,
}: {
  brand?: Brand;
  children: React.ReactNode;
  onCommand: () => void;
}) {
  const pathname = usePathname();
  const t = useTranslations("nav");
  const tu = useTranslations("userNav");
  const ta = useTranslations("admin");
  const tch = useTranslations("channelNav");
  const tp = useTranslations("partnerNav");
  const tc = useTranslations("chrome");
  const [navOpen, setNavOpen] = useState(false);
  const navTrigger = useRef<HTMLButtonElement>(null);
  const isAdmin = pathname.startsWith("/admin");
  const isUser = pathname.startsWith("/app") || pathname.startsWith("/console");
  const isChannel = pathname.startsWith("/channel");
  const isPartner = pathname.startsWith("/partner");
  const portalHref = isAdmin ? "/admin" : isChannel ? "/channel" : isPartner ? "/partner" : "/app";
  const portalKey = isAdmin ? "admin" : isChannel ? "channel" : isPartner ? "partner" : "app";
  const viewer = useViewer();
  useEffect(() => { if (!viewer.loading && viewer.signedIn) rememberConsoleWorkspace(viewer.userId, pathname, viewer.roles); }, [pathname, viewer.loading, viewer.signedIn, viewer.userId, viewer.roles]);
  const title = t(isChannel && viewer.channelType === "C" ? "oem" : portalKey);
  const adminNav = filterAdminGroups(adminGroups, viewer);

  const pageLabel = useMemo(() => {
    if (isUser) {
      const item = navItemForPath(pathname, [...userNavGroups.flatMap((group) => group.items), ...userSettingsNav]);
      return item ? tu(item.key) : title;
    }
    if (isChannel) {
      const item = channelNavItemForPath(pathname, viewer.channelType);
      return item ? (viewer.channelType === "C" ? ta : tch)(item.key) : title;
    }
    if (isPartner) {
      const item = navItemForPath(pathname, partnerNavGroups.flatMap((group) => group.items));
      return item ? tp(item.key) : title;
    }
    if (isAdmin) {
      const item = adminNav.flatMap((group) => group.items).find(item => adminNavActive(pathname, item.href) && item.href !== "/admin") || navItemForPath(pathname, adminNav.flatMap((group) => group.items));
      return item ? ta(item.key) : title;
    }
    return title;
  }, [adminNav, isAdmin, isChannel, isPartner, isUser, pathname, ta, tch, title, tp, tu, viewer.channelType]);

  return (
    <div className="flex min-h-screen min-w-0 flex-col" data-testid="console-shell">
      <header className="sticky top-0 z-30 border-b border-hairline bg-canvas">
        <div className="flex h-16 min-w-0 items-center gap-2 px-3 md:gap-4 md:px-6">
          <Button
            ref={navTrigger}
            type="button"
            variant="ghost"
            size="icon"
            className="shrink-0 md:hidden"
            aria-expanded={navOpen}
            aria-controls="console-nav-drawer"
            onClick={() => setNavOpen(true)}
          >
            <Menu />
            <span className="sr-only">{tc("openNav")}</span>
          </Button>
          <Link href={portalHref} aria-label={brand?.name || title} className="flex shrink-0 items-center gap-2.5 text-ink no-underline">
            <BrandLogo brand={brand} />
            <span className="hidden text-lg font-semibold tracking-tight md:inline">{brand?.name || title}</span>
          </Link>
          <p
            data-testid="console-page-title"
            className="min-w-0 flex-1 truncate text-sm font-medium text-ink"
          >
            {pageLabel}
          </p>
          <div className="ml-auto flex min-w-0 shrink-0 items-center gap-1">
            <div className="hidden items-center gap-1 md:flex" data-testid="console-chrome-inline">
              <LocaleSwitch />
              <ThemeToggle />
              <Button variant="ghost" size="sm" onClick={onCommand}>
                <Search />
                <span className="hidden lg:inline">{tc("jump")}</span>
              </Button>
              {!isUser ? (
                <Button asChild size="sm" variant="outline">
                  <Link href="/docs">{tc("docs")}</Link>
                </Button>
              ) : null}
            </div>
            <ConsoleOverflowMenu onCommand={onCommand} />
            {isUser ? <UserShellRightZone /> : null}
            {!isUser ? <UserShellRightZone variant="admin" /> : null}
          </div>
        </div>
      </header>
      <Dialog open={navOpen} onOpenChange={setNavOpen}>
        <DialogContent
          id="console-nav-drawer"
          className="left-0 top-0 h-dvh max-h-dvh w-[min(18rem,85vw)] max-w-none translate-x-0 translate-y-0 content-start overflow-y-auto rounded-none"
          onCloseAutoFocus={(event) => { event.preventDefault(); navTrigger.current?.focus(); }}
        >
          <DialogHeader className="text-left">
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription className="sr-only">{tc("openNav")}</DialogDescription>
          </DialogHeader>
          <nav aria-label={title}>
            <ConsoleNav pathname={pathname} onNavigate={() => setNavOpen(false)} />
          </nav>
        </DialogContent>
      </Dialog>
      <div className="flex min-w-0 flex-1 flex-col md:flex-row">
        <nav className="th-scrollbar hidden w-60 shrink-0 border-r border-hairline px-4 py-8 md:block" aria-label={title}>
          <ConsoleNav pathname={pathname} />
        </nav>
        <main className="min-w-0 flex-1 px-4 py-6 md:px-8 md:py-10">{children}</main>
      </div>
    </div>
  );
}
