import Link from "next/link";
import type { Brand } from "@/lib/brand";

export function SiteFooter({ brand }: { brand?: Brand }) {
  const name = brand?.name || "TokenHub";
  return (
    <footer className="border-t border-hairline bg-canvas">
      <div className="mx-auto flex max-w-[1120px] flex-col gap-3 px-6 py-8 text-[13px] text-ink-mute sm:flex-row sm:items-center sm:justify-between">
        <p>
          {name} Clearing · 纸/碳双主题 · 四个入口共用同一套语法
        </p>
        <p className="flex flex-wrap gap-3">
          <Link href="/app">用户台</Link>
          <Link href="/channel">渠道台</Link>
          <Link href="/admin">管理台</Link>
        </p>
      </div>
    </footer>
  );
}
