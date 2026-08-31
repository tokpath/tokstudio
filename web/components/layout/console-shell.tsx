"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import type { Brand } from "@/lib/brand";
import {
  adminGroups,
  channelNavGroups,
  isNavActive,
  partnerNavGroups,
  userNavGroups,
} from "@/lib/nav";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import { BrandMark } from "@/components/brand-mark";
import { LocaleSwitch } from "@/components/locale-switch";

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
        <div key={group.titleKey} className="mb-4 shrink-0">
          <p className="th-eyebrow mb-2 px-3 text-ink-mute">{t(group.titleKey)}</p>
          <ul className="flex gap-1 md:flex-col">
            {group.items.map((item) => {
              const active = isNavActive(pathname, item.href);
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className={`relative block shrink-0 rounded-control px-3 py-2 text-sm no-underline md:w-full ${
                      active ? "bg-brand-soft text-brand-emphasis" : "text-ink hover:bg-canvas-raised"
                    }`}
                  >
                    {active ? <span className="absolute inset-y-2 left-0 hidden w-0.5 bg-brand md:block" /> : null}
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
  const isUser = pathname.startsWith("/app");
  const isChannel = pathname.startsWith("/channel");
  const isPartner = pathname.startsWith("/partner");
  const portalHref = isAdmin ? "/admin" : isChannel ? "/channel" : isPartner ? "/partner" : "/app";
  const portalKey = isAdmin ? "admin" : isChannel ? "channel" : isPartner ? "partner" : "app";
  const title = t(portalKey);

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-30 border-b border-hairline bg-canvas">
        <div className="flex h-14 items-center gap-4 px-6">
          <Link href={portalHref} className="flex items-center gap-2 text-ink no-underline">
            <BrandMark />
            <span className="text-lg font-semibold">{title}</span>
          </Link>
          <div className="ml-auto flex items-center gap-2">
            <LocaleSwitch />
            <ThemeToggle />
            <Button variant="ghost" size="sm" onClick={onCommand}>
              {tc("jump")}
            </Button>
            <Button asChild size="sm" variant="outline">
              <Link href="/docs">{tc("docs")}</Link>
            </Button>
          </div>
        </div>
      </header>
      <div className="flex flex-1 flex-col md:flex-row">
        <nav
          className="th-scrollbar border-b border-hairline px-3 py-2 md:w-60 md:border-b-0 md:border-r md:py-6"
          aria-label={title}
        >
          {isUser ? <GroupedNav groups={userNavGroups} pathname={pathname} t={tu} /> : null}
          {isChannel ? <GroupedNav groups={channelNavGroups} pathname={pathname} t={tch} /> : null}
          {isPartner ? <GroupedNav groups={partnerNavGroups} pathname={pathname} t={tp} /> : null}
          {isAdmin
            ? adminGroups.map((group) => (
                <div key={group.titleKey} className="mb-4">
                  <p className="th-eyebrow mb-2 px-3 text-ink-mute">{ta(group.titleKey)}</p>
                  <ul className="flex flex-col gap-1">
                    {group.items.map((item) => {
                      const active = pathname === item.href;
                      return (
                        <li key={item.href}>
                          <Link
                            href={item.href}
                            className={`relative block rounded-control px-3 py-2 text-sm no-underline ${
                              active ? "bg-brand-soft text-brand-emphasis" : "text-ink hover:bg-canvas-raised"
                            }`}
                          >
                            {active ? <span className="absolute inset-y-2 left-0 hidden w-0.5 bg-brand md:block" /> : null}
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
        <main className="min-w-0 flex-1 px-6 py-8">{children}</main>
      </div>
    </div>
  );
}
