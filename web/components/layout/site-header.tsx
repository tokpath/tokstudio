"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import type { Brand } from "@/lib/brand";
import { MEGA_MENUS, TOP_LINKS } from "@/lib/mega-nav";
import { ChevronDown, LogIn, Search, UserPlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import { BrandLogo } from "@/components/brand-logo";
import { LocaleSwitch } from "@/components/locale-switch";
import { iconForHref, iconForMegaLink, iconForMegaMenu } from "@/lib/page-icons";

export function SiteHeader({ brand, onCommand }: { brand?: Brand; onCommand: () => void }) {
  const pathname = usePathname();
  const t = useTranslations("mega");
  const tc = useTranslations("chrome");
  const name = brand?.name || "TokenHub";
  const [openId, setOpenId] = useState<string | null>(null);
  const rootRef = useRef<HTMLElement>(null);

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (!rootRef.current?.contains(e.target as Node)) setOpenId(null);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpenId(null);
    }
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, []);

  useEffect(() => {
    setOpenId(null);
  }, [pathname]);

  function linkLabel(link: { labelKey?: string; literal?: string }) {
    return link.literal || (link.labelKey ? t(link.labelKey) : "");
  }

  const mobileLinks = [
    ...MEGA_MENUS.flatMap((m) => m.columns.flatMap((c) => c.links)),
    ...TOP_LINKS,
  ].filter((item, i, arr) => arr.findIndex((x) => x.href === item.href && linkLabel(x) === linkLabel(item)) === i);

  return (
    <header ref={rootRef} className="sticky top-0 z-40 border-b border-hairline bg-canvas">
      <div className="mx-auto flex h-16 max-w-[1200px] items-center gap-6 px-6">
        <Link href="/" className="flex items-center gap-2 text-ink no-underline">
          <BrandLogo brand={brand} />
          <span className="text-2xl font-semibold">{name}</span>
        </Link>

        <nav className="relative hidden flex-1 items-center gap-0.5 md:flex" aria-label={tc("publicNav")}>
          {MEGA_MENUS.map((menu) => {
            const open = openId === menu.id;
            const active =
              (menu.href && (pathname === menu.href || pathname.startsWith(`${menu.href}/`))) ||
              menu.columns.some((col) => col.links.some((l) => pathname === l.href || pathname.startsWith(`${l.href}/`)));
            const MenuIcon = iconForMegaMenu(menu.id);
            return (
              <div key={menu.id} className="relative">
                <button
                  type="button"
                  aria-expanded={open}
                  aria-haspopup="true"
                  onClick={() => setOpenId(open ? null : menu.id)}
                  onMouseEnter={() => setOpenId(menu.id)}
                  className={`inline-flex items-center gap-1.5 rounded-control px-3 py-1.5 text-sm ${
                    open || active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:text-ink"
                  }`}
                >
                  <MenuIcon className="size-3.5" strokeWidth={1.75} aria-hidden />
                  {t(menu.labelKey)}
                  <ChevronDown
                    className={`size-3.5 opacity-70 transition-transform ${open ? "rotate-180" : ""}`}
                    strokeWidth={1.75}
                    aria-hidden
                  />
                </button>
              </div>
            );
          })}
          {TOP_LINKS.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
            const Icon = iconForHref(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`inline-flex items-center gap-1.5 rounded-control px-3 py-1.5 text-sm no-underline ${
                  active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:text-ink"
                }`}
              >
                <Icon className="size-3.5" strokeWidth={1.75} aria-hidden />
                {t(item.labelKey)}
              </Link>
            );
          })}

          {openId ? (
            <div
              className="absolute left-0 top-full z-50 mt-1 w-[min(720px,calc(100vw-3rem))] rounded-card border border-hairline bg-canvas-raised p-4 shadow-[0_1px_2px_rgba(20,20,20,0.06)]"
              onMouseLeave={() => setOpenId(null)}
            >
              {MEGA_MENUS.filter((m) => m.id === openId).map((menu) => (
                <div key={menu.id} className="grid gap-6 sm:grid-cols-3">
                  {menu.columns.map((col) => (
                    <div key={col.titleKey}>
                      <p className="th-eyebrow text-ink-mute">{t(col.titleKey)}</p>
                      <ul className="mt-3 flex flex-col gap-1">
                        {col.links.map((link) => {
                          const Icon = iconForMegaLink(link);
                          return (
                            <li key={`${col.titleKey}-${link.href}-${link.labelKey || link.literal}`}>
                              <Link
                                href={link.href}
                                className="flex items-start gap-2 rounded-control px-2 py-1.5 no-underline hover:bg-brand-soft/60"
                                onClick={() => setOpenId(null)}
                              >
                                <Icon className="mt-0.5 size-4 shrink-0 text-brand-emphasis" strokeWidth={1.75} aria-hidden />
                                <span>
                                  <span className="block text-sm text-ink">{linkLabel(link)}</span>
                                  {link.hintKey ? <span className="mt-0.5 block text-[12px] text-ink-mute">{t(link.hintKey)}</span> : null}
                                </span>
                              </Link>
                            </li>
                          );
                        })}
                      </ul>
                    </div>
                  ))}
                </div>
              ))}
            </div>
          ) : null}
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <LocaleSwitch />
          <ThemeToggle />
          <button
            type="button"
            onClick={onCommand}
            className="hidden h-10 items-center gap-2 rounded-control border border-hairline px-3 text-[13px] text-ink-mute md:inline-flex"
          >
            <Search className="size-3.5" strokeWidth={1.75} aria-hidden />
            {tc("jump")}
            <kbd className="font-mono text-[11px]">⌘K</kbd>
          </button>
          <Button asChild variant="ghost" size="sm">
            <Link href="/login">
              <LogIn />
              {tc("login")}
            </Link>
          </Button>
          <Button asChild size="sm" className="hidden sm:inline-flex">
            <Link href="/login">
              <UserPlus />
              {tc("register")}
            </Link>
          </Button>
        </div>
      </div>

      <nav className="flex gap-1 overflow-x-auto border-t border-hairline px-6 py-2 md:hidden" aria-label={tc("mobileNav")}>
        {mobileLinks.slice(0, 12).map((item) => {
          const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
          return (
            <Link
              key={`${item.href}-${linkLabel(item)}`}
              href={item.href}
              className={`shrink-0 rounded-control px-3 py-1.5 text-sm no-underline ${
                active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary"
              }`}
            >
              {linkLabel(item)}
            </Link>
          );
        })}
      </nav>
    </header>
  );
}
