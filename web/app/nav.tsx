import Link from "next/link";
import type { Brand } from "@/lib/brand";

export function Nav({ brand }: { brand?: Brand }) {
  return (
    <header className="border-b border-slate-800 px-6 py-4">
      <div className="mx-auto flex max-w-5xl items-center justify-between gap-4">
        <Link href="/" className="text-lg font-semibold" style={{ color: "var(--brand-primary)" }}>
          {brand?.name || "TokenHub"}
        </Link>
        <nav className="flex flex-wrap gap-4 text-sm text-slate-300">
          <Link href="/">公共站</Link>
          <Link href="/docs">开发者文档</Link>
          <Link href="/app">用户控制台</Link>
          <Link href="/channel">渠道控制台</Link>
          <Link href="/admin">平台管理</Link>
          <Link href="/login">登录</Link>
        </nav>
      </div>
    </header>
  );
}
