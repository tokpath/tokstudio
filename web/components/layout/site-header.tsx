"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import type { Brand } from "@/lib/brand";
import { MEGA_MENUS, TOP_LINKS } from "@/lib/mega-nav";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import { BrandMark } from "@/components/brand-mark";

export function SiteHeader({ brand, onCommand }: { brand?: Brand; onCommand: () => void }) {
  const pathname = usePathname();
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

  const mobileLinks = [
    ...MEGA_MENUS.flatMap((m) => m.columns.flatMap((c) => c.links)),
    ...TOP_LINKS,
  ].filter((item, i, arr) => arr.findIndex((x) => x.href === item.href && x.label === item.label) === i);

  return (
    <header ref={rootRef} className="sticky top-0 z-40 border-b border-hairline bg-canvas">
      <div className="mx-auto flex h-14 max-w-[1120px] items-center gap-3 px-6">
        <Link href="/" className="flex items-center gap-2 text-ink no-underline">
          {brand?.logo_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={brand.logo_url} alt="" className="h-6 w-6 rounded-stamp object-contain" />
          ) : (
            <BrandMark />
          )}
          <span className="text-2xl font-semibold">{name}</span>
        </Link>

        <nav className="relative hidden flex-1 items-center gap-0.5 md:flex" aria-label="公共站">
          {MEGA_MENUS.map((menu) => {
            const open = openId === menu.id;
            const active =
              (menu.href && (pathname === menu.href || pathname.startsWith(`${menu.href}/`))) ||
              menu.columns.some((col) => col.links.some((l) => pathname === l.href || pathname.startsWith(`${l.href}/`)));
            return (
              <div key={menu.id} className="relative">
                <button
                  type="button"
                  aria-expanded={open}
                  aria-haspopup="true"
                  onClick={() => setOpenId(open ? null : menu.id)}
                  onMouseEnter={() => setOpenId(menu.id)}
                  className={`inline-flex items-center gap-1 rounded-control px-3 py-1.5 text-sm ${
                    open || active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:text-ink"
                  }`}
                >
                  {menu.label}
                  <span className="text-[10px] opacity-70">▾</span>
                </button>
              </div>
            );
          })}
          {TOP_LINKS.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`rounded-control px-3 py-1.5 text-sm no-underline ${
                  active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:text-ink"
                }`}
              >
                {item.label}
              </Link>
            );
          })}

          {openId ? (
            <div
              className="absolute left-0 top-full z-50 mt-1 w-[min(720px,calc(100vw-3rem))] rounded-stamp border border-hairline bg-canvas-raised p-4 shadow-[0_1px_2px_rgba(20,20,20,0.06)]"
              onMouseLeave={() => setOpenId(null)}
            >
              {MEGA_MENUS.filter((m) => m.id === openId).map((menu) => (
                <div key={menu.id} className="grid gap-6 sm:grid-cols-3">
                  {menu.columns.map((col) => (
                    <div key={col.title}>
                      <p className="th-eyebrow text-ink-mute">{col.title}</p>
                      <ul className="mt-3 flex flex-col gap-1">
                        {col.links.map((link) => (
                          <li key={`${col.title}-${link.href}-${link.label}`}>
                            <Link
                              href={link.href}
                              className="block rounded-control px-2 py-1.5 no-underline hover:bg-brand-soft/60"
                              onClick={() => setOpenId(null)}
                            >
                              <span className="text-sm text-ink">{link.label}</span>
                              {link.hint ? <span className="mt-0.5 block text-[12px] text-ink-mute">{link.hint}</span> : null}
                            </Link>
                          </li>
                        ))}
                      </ul>
                    </div>
                  ))}
                </div>
              ))}
            </div>
          ) : null}
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
          <button
            type="button"
            onClick={onCommand}
            className="hidden h-10 items-center gap-2 rounded-control border border-hairline px-3 text-[13px] text-ink-mute md:inline-flex"
          >
            跳转
            <kbd className="font-mono text-[11px]">⌘K</kbd>
          </button>
          <Button asChild variant="outline" size="sm">
            <Link href="/login">登录</Link>
          </Button>
          <Button asChild size="sm" className="hidden sm:inline-flex">
            <Link href="/login">注册</Link>
          </Button>
        </div>
      </div>

      <nav className="flex gap-1 overflow-x-auto border-t border-hairline px-6 py-2 md:hidden" aria-label="公共站移动导航">
        {mobileLinks.slice(0, 12).map((item) => {
          const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
          return (
            <Link
              key={`${item.href}-${item.label}`}
              href={item.href}
              className={`shrink-0 rounded-control px-3 py-1.5 text-sm no-underline ${
                active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary"
              }`}
            >
              {item.label}
            </Link>
          );
        })}
      </nav>
    </header>
  );
}
