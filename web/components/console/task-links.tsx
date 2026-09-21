"use client";
import Link from "next/link";
import { useTranslations } from "next-intl";
export function TaskLinks({ items }: { items: { id: string; href: string }[] }) {
  const t = useTranslations("console");
  return <div className="grid gap-4 md:grid-cols-2">{items.map(item => <Link key={item.id} href={item.href} className="rounded-card border border-hairline bg-canvas-raised p-5 hover:border-brand-emphasis focus-visible:outline focus-visible:outline-2">
    <h2 className="font-semibold">{t(`${item.id}.title`)} →</h2>
    <p className="mt-2 text-sm text-ink-secondary">{t(`${item.id}.description`)}</p>
  </Link>)}</div>;
}
