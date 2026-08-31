"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { Brand } from "@/lib/brand";
import { PUBLIC_NAV } from "@/lib/nav";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import { BrandMark } from "@/components/brand-mark";

export function SiteHeader({ brand, onCommand }: { brand?: Brand; onCommand: () => void }) {
  const pathname = usePathname();
  const name = brand?.name || "TokenHub";

  return (
    <header className="sticky top-0 z-40 border-b border-hairline bg-canvas">
      <div className="mx-auto flex h-16 max-w-[1200px] items-center gap-6 px-6">
        <Link href="/" className="flex items-center gap-2 text-ink no-underline">
          {brand?.logo_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={brand.logo_url} alt="" className="h-6 w-6 rounded-stamp object-contain" />
          ) : (
            <BrandMark />
          )}
          <span className="text-2xl font-semibold">{name}</span>
        </Link>
        <nav className="hidden flex-1 items-center gap-1 md:flex" aria-label="公共站">
          {PUBLIC_NAV.map((item) => {
            const active = item.href === "/" ? pathname === "/" : pathname === item.href;
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
          <Button asChild variant="ghost" size="sm">
            <Link href="/login">登录</Link>
          </Button>
          <Button asChild size="sm" className="hidden sm:inline-flex">
            <Link href="/login">开始使用</Link>
          </Button>
        </div>
      </div>
      <nav className="flex gap-1 overflow-x-auto border-t border-hairline px-6 py-2 md:hidden" aria-label="公共站移动导航">
        {PUBLIC_NAV.map((item) => {
          const active = item.href === "/" ? pathname === "/" : pathname === item.href;
          return (
            <Link
              key={item.href}
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
