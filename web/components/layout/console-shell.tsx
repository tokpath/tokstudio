"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  adminGroups,
  channelNavGroups,
  isNavActive,
  partnerNavGroups,
  userNavGroups,
} from "@/lib/nav";
import { adminNavActive } from "@/lib/tenants";
import { Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import type { Brand } from "@/lib/brand";
import { BrandLogo } from "@/components/brand-logo";
import { LocaleSwitch } from "@/components/locale-switch";
import { iconForHref } from "@/lib/page-icons";

function GroupedNav({
  groups,
  pathname,
  t,
}: {
  groups: { titleKey: string; items: { href: string; key: string }[] }[];
  pathname: string;
  t: (key: string) => string;
}) {
  return (
    <div className="flex gap-4 overflow-x-auto md:flex-col md:overflow-visible md:gap-0">
      {groups.map((group) => (
        <div key={group.titleKey} className="mb-6 shrink-0">
          <p className="th-eyebrow mb-2.5 px-3 text-ink-mute">{t(group.titleKey)}</p>
          <ul className="flex gap-1 md:flex-col md:gap-0.5">
            {group.items.map((item) => {
              const active = isNavActive(pathname, item.href);
              const Icon = iconForHref(item.href);
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className={`relative flex shrink-0 items-center gap-2.5 rounded-control px-3 py-2 text-sm no-underline transition-colors duration-150 md:w-full ${
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
  const ta = useTranslations("admin");
  const tu = useTranslations("userNav");
  const tch = useTranslations("channelNav");
  const tp = useTranslations("partnerNav");
  const tc = useTranslations("chrome");
  const isAdmin = pathname.startsWith("/admin");
  const isUser = pathname.startsWith("/app") || pathname.startsWith("/console");
  const isChannel = pathname.startsWith("/channel");
  const isPartner = pathname.startsWith("/partner");
  const portalHref = isAdmin ? "/admin" : isChannel ? "/channel" : isPartner ? "/partner" : "/app";
  const portalKey = isAdmin ? "admin" : isChannel ? "channel" : isPartner ? "partner" : "app";
  const title = t(portalKey);

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-30 border-b border-hairline bg-canvas">
        <div className="flex h-16 items-center gap-4 px-6">
          <Link href={portalHref} className="flex items-center gap-2.5 text-ink no-underline">
            <BrandLogo brand={brand} />
            <span className="text-lg font-semibold tracking-tight">{brand?.name || title}</span>
          </Link>
          <div className="ml-auto flex items-center gap-1">
            <LocaleSwitch />
            <ThemeToggle />
            <Button variant="ghost" size="sm" onClick={onCommand}>
              <Search />
              {tc("jump")}
            </Button>
            <Button asChild size="sm" variant="outline">
              <Link href="/docs">
                {tc("docs")}
              </Link>
            </Button>
          </div>
        </div>
      </header>
      <div className="flex flex-1 flex-col md:flex-row">
        <nav
          className="th-scrollbar border-b border-hairline px-3 py-3 md:w-60 md:border-b-0 md:border-r md:px-4 md:py-8"
          aria-label={title}
        >
          {isUser ? <GroupedNav groups={userNavGroups} pathname={pathname} t={tu} /> : null}
          {isChannel ? <GroupedNav groups={channelNavGroups} pathname={pathname} t={tch} /> : null}
          {isPartner ? <GroupedNav groups={partnerNavGroups} pathname={pathname} t={tp} /> : null}
          {isAdmin
            ? adminGroups.map((group) => (
                <div key={group.titleKey} className="mb-6">
                  <p className="th-eyebrow mb-2.5 px-3 text-ink-mute">{ta(group.titleKey)}</p>
                  <ul className="flex flex-col gap-0.5">
                    {group.items.map((item) => {
                      const active = adminNavActive(pathname, item.href);
                      const Icon = iconForHref(item.href);
                      return (
                        <li key={item.href}>
                          <Link
                            href={item.href}
                            className={`relative flex items-center gap-2.5 rounded-control px-3 py-2 text-sm no-underline transition-colors duration-150 ${
                              active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:bg-canvas-raised hover:text-ink"
                            }`}
                          >
                            {active ? <span className="absolute inset-y-2 left-0 hidden w-0.5 bg-brand md:block" /> : null}
                            <Icon className="size-4 shrink-0" strokeWidth={1.75} aria-hidden />
                            {ta(item.key)}
                          </Link>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ))
            : null}
        </nav>
        <main className="min-w-0 flex-1 px-6 py-8 md:px-8 md:py-10">{children}</main>
      </div>
    </div>
  );
}
