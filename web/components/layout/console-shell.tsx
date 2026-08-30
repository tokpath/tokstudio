"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import type { Brand } from "@/lib/brand";
import { adminGroups, channelSections, partnerSections, portalLinks, userSections } from "@/lib/nav";
import { Button } from "@/components/ui/button";

function SectionLinks({ items, pathname }: { items: { href: string; label: string }[]; pathname: string }) {
  return (
    <ul className="space-y-1">
      {items.map((item) => (
        <li key={item.href}>
          <a href={item.href} className="block rounded-lg px-3 py-1.5 text-sm text-slate-300 hover:bg-white/5 hover:text-white">
            {item.label}
          </a>
        </li>
      ))}
      {pathname ? null : null}
    </ul>
  );
}

export function ConsoleShell({
  brand,
  children,
  onCommand,
}: {
  brand?: Brand;
  children: React.ReactNode;
  onCommand: () => void;
}) {
  const pathname = usePathname();
  const t = useTranslations("nav");
  const ta = useTranslations("admin");
  const isAdmin = pathname.startsWith("/admin");
  const isUser = pathname.startsWith("/app");
  const isChannel = pathname.startsWith("/channel");
  const isPartner = pathname.startsWith("/partner");

  return (
    <div className="min-h-screen lg:grid lg:grid-cols-[260px_1fr]">
      <aside className="th-scrollbar border-b border-white/10 bg-[#07111f]/90 lg:sticky lg:top-0 lg:h-screen lg:overflow-y-auto lg:border-b-0 lg:border-r">
        <div className="flex items-center justify-between px-4 py-4">
          <Link href="/" className="text-base font-semibold" style={{ color: "var(--brand-primary)" }}>
            {brand?.name || "TokenHub"}
          </Link>
          <button type="button" onClick={onCommand} className="rounded-md border border-white/10 px-2 py-1 text-[10px] text-slate-400">
            ⌘K
          </button>
        </div>
        <div className="px-3 pb-6">
          <p className="mb-2 px-3 text-[11px] uppercase tracking-[0.16em] text-slate-500">门户</p>
          <ul className="mb-5 space-y-1">
            {portalLinks.map((item) => {
              const active = item.href === "/" ? pathname === "/" : pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className={`block rounded-lg px-3 py-1.5 text-sm ${active ? "bg-white/8 text-white" : "text-slate-300 hover:bg-white/5 hover:text-white"}`}
                  >
                    {t(item.key)}
                  </Link>
                </li>
              );
            })}
          </ul>
          {isUser ? (
            <>
              <p className="mb-2 px-3 text-[11px] uppercase tracking-[0.16em] text-slate-500">本页</p>
              <SectionLinks items={userSections} pathname={pathname} />
            </>
          ) : null}
          {isChannel ? (
            <>
              <p className="mb-2 px-3 text-[11px] uppercase tracking-[0.16em] text-slate-500">本页</p>
              <SectionLinks items={channelSections} pathname={pathname} />
            </>
          ) : null}
          {isPartner ? (
            <>
              <p className="mb-2 px-3 text-[11px] uppercase tracking-[0.16em] text-slate-500">本页</p>
              <SectionLinks items={partnerSections} pathname={pathname} />
            </>
          ) : null}
          {isAdmin
            ? adminGroups.map((group) => (
                <div key={group.title} className="mb-4">
                  <p className="mb-2 px-3 text-[11px] uppercase tracking-[0.16em] text-slate-500">{group.title}</p>
                  <ul className="space-y-1">
                    {group.items.map((item) => {
                      const active = pathname === item.href;
                      return (
                        <li key={item.href}>
                          <Link
                            href={item.href}
                            className={`block rounded-lg px-3 py-1.5 text-sm ${active ? "bg-white/8 text-white" : "text-slate-300 hover:bg-white/5 hover:text-white"}`}
                          >
                            {ta(item.key)}
                          </Link>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ))
            : null}
        </div>
      </aside>
      <div className="min-w-0">
        <div className="sticky top-0 z-30 flex items-center justify-between gap-3 border-b border-white/10 bg-[#020617]/70 px-4 py-3 backdrop-blur-xl md:px-6">
          <p className="text-sm text-slate-400">
            {isAdmin ? "平台管理控制台" : isChannel ? "渠道控制台" : isPartner ? "分销控制台" : "用户控制台"}
          </p>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={onCommand}>
              快速跳转
            </Button>
            <Button asChild size="sm" variant="outline">
              <Link href="/docs">文档</Link>
            </Button>
          </div>
        </div>
        <div className="px-4 py-8 md:px-8">{children}</div>
      </div>
    </div>
  );
}
