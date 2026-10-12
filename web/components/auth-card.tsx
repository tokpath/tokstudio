"use client";

import Link from "next/link";
import type { ReactNode } from "react";
import { useBrand } from "@/components/brand-context";
import { BrandLogo } from "@/components/brand-logo";
import { LocaleSwitch } from "@/components/locale-switch";
import { ThemeToggle } from "@/components/theme-toggle";

export function AuthCard({ title, children }: { title: string; children: ReactNode }) {
  const brand = useBrand();
  return <main className="flex min-h-svh flex-col items-center justify-center px-6 py-12">
    <div className="w-full max-w-[28rem]">
      <Link href="/" className="mb-8 flex items-center justify-center gap-2.5 text-ink"><BrandLogo brand={brand} /><span className="text-xl font-semibold">{brand?.name || "TokenHub"}</span></Link>
      <section className="rounded-card border border-hairline bg-canvas-raised p-7 sm:p-9">
        <div className="mb-6 flex items-start justify-between gap-3"><h1 className="text-2xl font-semibold">{title}</h1><div className="flex shrink-0"><LocaleSwitch /><ThemeToggle /></div></div>
        {children}
      </section>
    </div>
  </main>;
}
