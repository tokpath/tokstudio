"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { userSettingsNav } from "@/lib/nav";

export function SettingsSubnav() {
  const pathname = usePathname();
  const t = useTranslations("userNav");
  return (
    <nav aria-label={t("settings")} className="flex flex-wrap gap-1 border-b border-hairline pb-3">
      {userSettingsNav.map((item) => {
        const active = pathname === item.href;
        return (
          <Link
            key={item.href}
            href={item.href}
            className={`rounded-stamp px-3 py-2 text-sm no-underline ${
              active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:bg-canvas-raised hover:text-ink"
            }`}
          >
            {t(item.key)}
          </Link>
        );
      })}
    </nav>
  );
}
