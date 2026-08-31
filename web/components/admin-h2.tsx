"use client";

import { useTranslations } from "next-intl";

export function AdminH2({
  k,
  className = "mb-3 text-xl font-medium",
}: {
  k: string;
  className?: string;
}) {
  const t = useTranslations("adminUi");
  return <h2 className={className}>{t(k)}</h2>;
}
