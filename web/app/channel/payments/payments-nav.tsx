"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { iconForHref } from "@/lib/page-icons";

const items = [
  { href: "/channel/payments", key: "lanes" },
  { href: "/channel/payments/rules", key: "rules" },
  { href: "/channel/payments/orders", key: "orders" },
];

export function ChannelPaymentsNav() {
  const pathname = usePathname();
  const t = useTranslations("channelPayments");
  return (
    <nav aria-label={t("nav")} className="flex flex-wrap gap-1 border-b border-hairline pb-3">
      {items.map((item) => {
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
