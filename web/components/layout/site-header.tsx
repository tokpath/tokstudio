"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import type { Brand } from "@/lib/brand";
import { portalLinks } from "@/lib/nav";
import { Button } from "@/components/ui/button";

export function SiteHeader({ brand, onCommand }: { brand?: Brand; onCommand: () => void }) {
  const t = useTranslations("nav");
  const pathname = usePathname();
  return (
    <header className="sticky top-0 z-40 border-b border-white/10 bg-[#020617]/75 backdrop-blur-xl">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-6">
        <Link href="/" className="flex items-center gap-2 text-lg font-semibold tracking-tight" style={{ color: "var(--brand-primary)" }}>
          <span className="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-[color-mix(in_srgb,var(--brand-primary)_16%,transparent)] text-sm">
            {(brand?.name || "T").slice(0, 1)}
          </span>
          {brand?.name || "TokenHub"}
        </Link>
        <nav className="hidden items-center gap-1 text-sm text-slate-300 lg:flex">
          {portalLinks.map((item) => {
            const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`rounded-full px-3 py-1.5 transition-colors hover:bg-white/5 hover:text-white ${active ? "bg-white/8 text-white" : ""}`}
              >
                {t(item.key)}
              </Link>
            );
          })}
        </nav>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onCommand}
            className="hidden h-9 items-center gap-2 rounded-full border border-white/10 bg-white/5 px-3 text-xs text-slate-400 md:inline-flex"
          >
            搜索
            <kbd className="rounded bg-black/40 px-1.5 py-0.5 font-mono text-[10px]">⌘K</kbd>
          </button>
          <Button asChild size="sm">
            <Link href="/login">注册</Link>
          </Button>
        </div>
      </div>
      <nav className="flex flex-wrap gap-2 border-t border-white/5 px-4 py-2 text-xs text-slate-400 lg:hidden">
        {portalLinks.map((item) => (
          <Link key={item.href} href={item.href} className="rounded-full border border-white/10 px-2.5 py-1">
            {t(item.key)}
          </Link>
        ))}
      </nav>
    </header>
  );
}
