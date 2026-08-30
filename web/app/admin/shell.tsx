"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";

const links = [
  { href: "/admin", key: "overview" },
  { href: "/admin/providers", key: "providers" },
  { href: "/admin/models", key: "models" },
  { href: "/admin/routes", key: "routes" },
  { href: "/admin/keys", key: "keys" },
  { href: "/admin/users", key: "users" },
  { href: "/admin/plans", key: "plans" },
  { href: "/admin/prices", key: "prices" },
  { href: "/admin/payments", key: "payments" },
  { href: "/admin/billing", key: "billing" },
  { href: "/admin/metrics", key: "metrics" },
  { href: "/admin/usage", key: "usage" },
  { href: "/admin/media", key: "media" },
  { href: "/admin/channels", key: "channels" },
  { href: "/admin/promos", key: "promos" },
  { href: "/admin/commission", key: "commission" },
  { href: "/admin/alerts", key: "alerts" },
  { href: "/admin/runbooks", key: "runbooks" },
  { href: "/admin/audit", key: "audit" },
  { href: "/admin/settings", key: "settings" },
] as const;

export function AdminShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const t = useTranslations("admin");
  return (
    <main className="mx-auto flex max-w-6xl flex-col gap-4 px-6 py-10">
      <p className="text-sm uppercase tracking-[0.2em] text-slate-400">平台管理控制台</p>
      <h1 className="text-3xl font-semibold">{t("title")}</h1>
      <nav className="flex flex-wrap gap-2 text-sm">
        {links.map((link) => {
          const active = pathname === link.href;
          return (
            <Link
              key={link.href}
              href={link.href}
              className={`rounded-full border px-3 py-1 ${active ? "border-cyan-400 text-cyan-200" : "border-slate-700 text-slate-300"}`}
            >
              {t(link.key)}
            </Link>
          );
        })}
      </nav>
      {children}
    </main>
  );
}
