"use client";

import { useTranslations } from "next-intl";

export function AdminShell({ children }: { children: React.ReactNode }) {
  const t = useTranslations("admin");
  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-6">
      <header>
        <p className="text-sm uppercase tracking-[0.2em] text-slate-400">平台管理控制台</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">{t("title")}</h1>
      </header>
      {children}
    </div>
  );
}
