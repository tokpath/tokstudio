import Link from "next/link";
import type { Brand } from "@/lib/brand";
import { FOOTER_GROUPS } from "@/lib/public-site";

export function SiteFooter({ brand }: { brand?: Brand }) {
  const name = brand?.name || "TokenHub";
  return (
    <footer className="border-t border-hairline bg-canvas">
      <div className="mx-auto grid max-w-[1120px] gap-8 px-6 py-12 md:grid-cols-4">
        <div>
          <p className="text-lg font-semibold text-ink">{name}</p>
          <p className="mt-2 text-[13px] text-ink-mute">
            一张已发布的价目单，外加一枚签核章。纸/碳双主题，四个入口共用同一套语法。
          </p>
        </div>
        {FOOTER_GROUPS.map((group) => (
          <div key={group.title}>
            <p className="th-eyebrow text-ink-mute">{group.title}</p>
            <ul className="mt-3 flex flex-col gap-2 text-[13px] text-ink-secondary">
              {group.links.map((link) => (
                <li key={link.href}>
                  <Link href={link.href} className="no-underline hover:text-ink">
                    {link.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <div className="border-t border-hairline">
        <div className="mx-auto flex max-w-[1120px] flex-col gap-2 px-6 py-4 text-[13px] text-ink-mute sm:flex-row sm:items-center sm:justify-between">
          <p>© {new Date().getFullYear()} {name} Clearing</p>
          <p className="flex flex-wrap gap-3">
            <Link href="/app">用户台</Link>
            <Link href="/channel">渠道台</Link>
            <Link href="/admin">管理台</Link>
            <Link href="/login">登录</Link>
          </p>
        </div>
      </div>
    </footer>
  );
}
