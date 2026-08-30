"use client";

import { usePathname } from "next/navigation";
import { BrandMark } from "@/components/brand-mark";
import { Button } from "@/components/button";
import { ThemeToggle } from "@/components/theme-toggle";
import { PUBLIC_NAV } from "@/lib/nav";

export function PublicHeader() {
  const pathname = usePathname();

  return (
    <header className="sticky top-0 z-10 border-b border-hairline bg-canvas">
      <div className="mx-auto flex h-14 max-w-[1120px] items-center gap-4 px-6">
        <a href="/" className="flex items-center gap-2 text-ink no-underline">
          <BrandMark />
          <span className="text-2xl font-semibold">TokenHub</span>
        </a>
        <nav className="hidden flex-1 items-center gap-1 md:flex" aria-label="公共站">
          {PUBLIC_NAV.map((item) => {
            const active = pathname === item.href;
            return (
              <a
                key={item.href}
                href={item.href}
                className={`rounded-control px-3 py-1.5 text-sm no-underline ${
                  active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:text-ink"
                }`}
              >
                {item.label}
              </a>
            );
          })}
        </nav>
        <div className="ml-auto flex items-center gap-3">
          <ThemeToggle />
          <Button href="/login" variant="secondary">
            登录
          </Button>
          <Button href="/register" variant="primary" className="hidden sm:inline-flex">
            开始使用
          </Button>
        </div>
      </div>
      <nav className="flex gap-1 overflow-x-auto border-t border-hairline px-6 py-2 md:hidden" aria-label="公共站移动导航">
        {PUBLIC_NAV.map((item) => {
          const active = pathname === item.href;
          return (
            <a
              key={item.href}
              href={item.href}
              className={`shrink-0 rounded-control px-3 py-1.5 text-sm no-underline ${
                active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary"
              }`}
            >
              {item.label}
            </a>
          );
        })}
      </nav>
    </header>
  );
}
