import Link from "next/link";
import type { Brand } from "@/lib/brand";

export function SiteFooter({ brand }: { brand?: Brand }) {
  const name = brand?.name || "TokenHub";
  return (
    <footer className="border-t border-white/10 bg-[#020617]/80">
      <div className="mx-auto grid max-w-6xl gap-8 px-6 py-12 md:grid-cols-4">
        <div className="md:col-span-1">
          <p className="text-lg font-semibold" style={{ color: "var(--brand-primary)" }}>
            {name}
          </p>
          <p className="mt-2 text-sm text-slate-400">一个 Base URL、一把 Key，接入多模型。账单可审计，渠道归因写死不可自改。</p>
        </div>
        <div>
          <p className="mb-3 text-xs font-medium uppercase tracking-[0.16em] text-slate-500">产品</p>
          <ul className="space-y-2 text-sm text-slate-300">
            <li>
              <Link href="/#models">模型目录</Link>
            </li>
            <li>
              <Link href="/#plans">套餐与订阅</Link>
            </li>
            <li>
              <Link href="/docs">开发者文档</Link>
            </li>
          </ul>
        </div>
        <div>
          <p className="mb-3 text-xs font-medium uppercase tracking-[0.16em] text-slate-500">控制台</p>
          <ul className="space-y-2 text-sm text-slate-300">
            <li>
              <Link href="/app">用户控制台</Link>
            </li>
            <li>
              <Link href="/channel">渠道控制台</Link>
            </li>
            <li>
              <Link href="/partner">分销控制台</Link>
            </li>
            <li>
              <Link href="/admin">平台管理</Link>
            </li>
          </ul>
        </div>
        <div>
          <p className="mb-3 text-xs font-medium uppercase tracking-[0.16em] text-slate-500">接入</p>
          <ul className="space-y-2 text-sm text-slate-300">
            <li>OpenAI Chat / Responses</li>
            <li>Anthropic Messages</li>
            <li>图像 / 视频任务</li>
          </ul>
        </div>
      </div>
      <div className="border-t border-white/5 px-6 py-4 text-center text-xs text-slate-500">
        © {new Date().getFullYear()} {name}. 金额单位 micro-USD。OEM 站点会替换品牌名和配色。
      </div>
    </footer>
  );
}
