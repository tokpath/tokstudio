"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  adminGroups,
  channelNavGroups,
  isNavActive,
  navItemForPath,
  partnerNavGroups,
  userNavGroups,
  userSettingsNav,
  type NavItem,
} from "@/lib/nav";
import { adminNavActive } from "@/lib/tenants";
import { Menu, Search } from "lucide-react";
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
import { UserShellBell, UserShellRightZone } from "@/components/layout/user-shell-menu";
import { iconForHref } from "@/lib/page-icons";
import { canAccessChannelPortal, canAccessPartnerPortal, filterAdminGroups, shouldBypassRbac } from "@/lib/rbac";
import { useViewer } from "@/components/rbac/viewer-context";

const navLinkFocus =
  "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand";

function GroupedNav({
  groups,
  pathname,
  t,
  onNavigate,
  isActive = isNavActive,
}: {
  groups: { titleKey: string; items: NavItem[] }[];
  pathname: string;
  t: (key: string) => string;
  onNavigate?: () => void;
  isActive?: (pathname: string, href: string) => boolean;
}) {
  return (
    <div className="flex flex-col gap-0">
      {groups.map((group) => (
        <div key={group.titleKey} className="mb-6">
          <p className="th-eyebrow mb-2.5 px-3 text-ink-mute">{t(group.titleKey)}</p>
          <ul className="flex flex-col gap-0.5">
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
      {isUser ? <GroupedNav groups={userNavGroups} pathname={pathname} t={tu} onNavigate={onNavigate} /> : null}
      {showChannelNav ? <GroupedNav groups={channelNavGroups} pathname={pathname} t={tch} onNavigate={onNavigate} /> : null}
      {showPartnerNav ? <GroupedNav groups={partnerNavGroups} pathname={pathname} t={tp} onNavigate={onNavigate} /> : null}
      {isAdmin ? (
        <GroupedNav groups={adminNav} pathname={pathname} t={ta} onNavigate={onNavigate} isActive={adminNavActive} />
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
  const isAdmin = pathname.startsWith("/admin");
  const isUser = pathname.startsWith("/app") || pathname.startsWith("/console");
  const isChannel = pathname.startsWith("/channel");
  const isPartner = pathname.startsWith("/partner");
  const portalHref = isAdmin ? "/admin" : isChannel ? "/channel" : isPartner ? "/partner" : "/app";
  const portalKey = isAdmin ? "admin" : isChannel ? "channel" : isPartner ? "partner" : "app";
  const title = t(portalKey);
  const viewer = useViewer();
  const adminNav = filterAdminGroups(adminGroups, viewer);

  const pageLabel = useMemo(() => {
    if (isUser) {
      const item = navItemForPath(pathname, [...userNavGroups.flatMap((group) => group.items), ...userSettingsNav]);
      return item ? tu(item.key) : title;
    }
    if (isChannel) {
      const item = navItemForPath(pathname, channelNavGroups.flatMap((group) => group.items));
      return item ? tch(item.key) : title;
    }
    if (isPartner) {
      const item = navItemForPath(pathname, partnerNavGroups.flatMap((group) => group.items));
      return item ? tp(item.key) : title;
    }
    if (isAdmin) {
      const item = navItemForPath(pathname, adminNav.flatMap((group) => group.items));
      return item ? ta(item.key) : title;
    }
    return title;
  }, [adminNav, isAdmin, isChannel, isPartner, isUser, pathname, ta, tch, title, tp, tu]);

  return (
    <div className="flex min-h-screen min-w-0 flex-col" data-testid="console-shell">
      <header className="sticky top-0 z-30 border-b border-hairline bg-canvas">
        <div className="flex h-16 min-w-0 items-center gap-2 px-3 md:gap-4 md:px-6">
          <Button
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
          <Link href={portalHref} className="flex shrink-0 items-center gap-2.5 text-ink no-underline">
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
              {isUser ? <UserShellBell /> : null}
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
            <ConsoleOverflowMenu onCommand={onCommand} showBell={isUser} />
            {isUser ? <UserShellRightZone /> : null}
            {!isUser ? <UserShellRightZone variant="admin" /> : null}
          </div>
        </div>
      </header>
      <Dialog open={navOpen} onOpenChange={setNavOpen}>
        <DialogContent
          id="console-nav-drawer"
          className="left-0 top-0 h-dvh max-h-dvh w-[min(18rem,85vw)] max-w-none translate-x-0 translate-y-0 overflow-y-auto rounded-none"
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
