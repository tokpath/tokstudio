"use client";

import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import { BrandMark } from "@/components/brand-mark";
import { ThemeToggle } from "@/components/theme-toggle";
import { PORTALS, type PortalId } from "@/lib/nav";

export function ConsoleShell({ portal, children }: { portal: PortalId; children: ReactNode }) {
  const pathname = usePathname();
  const config = PORTALS[portal];

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-10 border-b border-hairline bg-canvas">
        <div className="flex h-14 items-center gap-4 px-6">
          <a href="/" className="flex items-center gap-2 text-ink no-underline">
            <BrandMark />
            <span className="text-lg font-semibold">{config.name}</span>
          </a>
          <div className="ml-auto">
            <ThemeToggle />
          </div>
        </div>
      </header>
      <div className="flex flex-1 flex-col md:flex-row">
        <nav
          className="flex gap-1 overflow-x-auto border-b border-hairline px-4 py-2 md:w-60 md:flex-col md:overflow-visible md:border-b-0 md:border-r md:px-3 md:py-6"
          aria-label={config.name}
        >
          {config.items.map((item) => {
            const active = pathname === item.href;
            return (
              <a
                key={item.href}
                href={item.href}
                className={`relative shrink-0 rounded-control px-3 py-2 text-sm no-underline md:w-full ${
                  active ? "bg-brand-soft text-brand-emphasis" : "text-ink hover:bg-canvas-raised"
                }`}
              >
                {active ? <span className="absolute inset-y-2 left-0 hidden w-0.5 bg-brand md:block" /> : null}
                {item.label}
              </a>
            );
          })}
        </nav>
        <main className="flex-1 px-6 py-8">{children}</main>
      </div>
    </div>
  );
}
