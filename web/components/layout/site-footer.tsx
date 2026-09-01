"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import type { Brand } from "@/lib/brand";
import { FOOTER_GROUPS } from "@/lib/public-site";

export function SiteFooter({ brand }: { brand?: Brand }) {
  const name = brand?.name || "TokenHub";
  const t = useTranslations("footer");
  const tc = useTranslations("chrome");
  return (
    <footer className="border-t border-hairline bg-canvas">
      <div className="mx-auto grid max-w-[1200px] gap-12 px-6 py-14 sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <p className="text-lg font-semibold tracking-tight text-ink">{name}</p>
          <p className="mt-3 max-w-xs text-[13px] leading-relaxed text-ink-mute">{tc("footerTagline")}</p>
        </div>
        {FOOTER_GROUPS.map((group) => (
          <div key={group.titleKey}>
            <p className="th-eyebrow text-ink-mute">{t(group.titleKey)}</p>
            <ul className="mt-4 flex flex-col gap-2.5 text-[13px] text-ink-secondary">
              {group.links.map((link) => (
                <li key={link.href}>
                  <Link href={link.href} className="no-underline transition-colors duration-150 hover:text-ink">
                    {t(link.labelKey)}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <div className="border-t border-hairline">
        <div className="mx-auto flex max-w-[1200px] flex-col gap-3 px-6 py-5 text-[13px] text-ink-mute sm:flex-row sm:items-center sm:justify-between">
          <p>
            © {new Date().getFullYear()} {name} {tc("footerCopy")}
          </p>
          <p className="flex flex-wrap gap-x-4 gap-y-2">
            <Link href="/app" className="transition-colors duration-150 hover:text-ink">{tc("footerApp")}</Link>
            <Link href="/channel" className="transition-colors duration-150 hover:text-ink">{tc("footerChannel")}</Link>
            <Link href="/admin" className="transition-colors duration-150 hover:text-ink">{tc("footerAdmin")}</Link>
            <Link href="/login" className="transition-colors duration-150 hover:text-ink">{tc("login")}</Link>
          </p>
        </div>
      </div>
    </footer>
  );
}
