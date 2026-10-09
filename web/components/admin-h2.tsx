"use client";

import { useTranslations } from "next-intl";

export function AdminH2({
  k,
  className = "mb-4 text-lg font-semibold tracking-tight",
  level = 2,
}: {
  k: string;
  className?: string;
  level?: 1 | 2;
}) {
  const t = useTranslations("adminUi");
  const Heading = level === 1 ? "h1" : "h2";
  return <Heading className={className}>{t(k)}</Heading>;
}
