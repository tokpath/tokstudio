"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { userSettingsNav } from "@/lib/nav";
import { iconForHref } from "@/lib/page-icons";

export function SettingsSubnav() {
  const pathname = usePathname();
  const t = useTranslations("userNav");
  return (
    <nav aria-label={t("settings")} className="flex flex-wrap gap-1 border-b border-hairline pb-3">
      {userSettingsNav.map((item) => {
        const active = pathname === item.href;
        const Icon = iconForHref(item.href);
        return (
          <Link
            key={item.href}
            href={item.href}
            className={`inline-flex items-center gap-1.5 rounded-stamp px-3 py-2 text-sm no-underline ${
              active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary hover:bg-canvas-raised hover:text-ink"
            }`}
          >
            <Icon className="size-3.5" strokeWidth={1.75} aria-hidden />
            {t(item.key)}
          </Link>
        );
      })}
    </nav>
  );
}
