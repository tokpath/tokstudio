"use client";

import { useTranslations } from "next-intl";

export function AdminShell({ children }: { children: React.ReactNode }) {
  const t = useTranslations("admin");
  return (
    <div className="flex w-full flex-col gap-6">
      <header>
        <p className="th-eyebrow text-ink-mute">ADMIN</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">{t("title")}</h1>
      </header>
      {children}
    </div>
  );
}
