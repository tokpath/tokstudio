"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import type { Brand } from "@/lib/brand";

export function Nav({ brand }: { brand?: Brand }) {
  const t = useTranslations("nav");
  return (
    <header className="border-b border-slate-800 px-6 py-4">
      <div className="mx-auto flex max-w-5xl items-center justify-between gap-4">
        <Link href="/" className="text-lg font-semibold" style={{ color: "var(--brand-primary)" }}>
          {brand?.name || "TokenHub"}
        </Link>
        <nav className="flex flex-wrap gap-4 text-sm text-slate-300">
          <Link href="/">{t("public")}</Link>
          <Link href="/docs">{t("docs")}</Link>
          <Link href="/app">{t("app")}</Link>
          <Link href="/channel">{t("channel")}</Link>
          <Link href="/admin">{t("admin")}</Link>
          <Link href="/login">{t("login")}</Link>
        </nav>
      </div>
    </header>
  );
}
