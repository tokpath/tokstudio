"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { userSettingsNav } from "@/lib/nav";
import { iconForHref } from "@/lib/page-icons";

export function SettingsSubnav() {
  const pathname = usePathname();
  const t = useTranslations("userNav");
  const tc = useTranslations("chrome");
  return (
    <nav aria-label={t("settings")} className="flex flex-wrap gap-1 border-b border-hairline pb-4">
      {userSettingsNav.map((item) => {
        const active = pathname === item.href;
        const Icon = iconForHref(item.href);
        const label = t(item.key);
        const className = `inline-flex min-h-11 items-center gap-1.5 rounded-stamp px-3 py-2 text-sm transition-colors duration-150 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand ${
          active ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary"
        }`;
        if (item.unavailable) {
          return (
            <span
              key={item.href}
              className={`${className} cursor-not-allowed opacity-70`}
              aria-disabled="true"
              title={tc("unavailableTitle")}
            >
              <Icon className="size-3.5" strokeWidth={1.75} aria-hidden />
              {label}
              <span className="rounded-stamp bg-canvas px-1.5 py-0.5 text-[11px] font-medium text-ink-mute">{tc("unavailable")}</span>
            </span>
          );
        }
        return (
          <Link key={item.href} href={item.href} className={`${className} no-underline hover:bg-canvas-raised hover:text-ink`}>
            <Icon className="size-3.5" strokeWidth={1.75} aria-hidden />
            {label}
          </Link>
        );
      })}
    </nav>
  );
}
