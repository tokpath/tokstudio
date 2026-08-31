import Link from "next/link";
import type { Brand } from "@/lib/brand";

const columns = [
  {
    title: "产品",
    links: [
      { href: "/#models", label: "价目" },
      { href: "/#plans", label: "套餐" },
      { href: "/docs", label: "快速开始" },
    ],
  },
  {
    title: "入口",
    links: [
      { href: "/app", label: "用户台" },
      { href: "/channel", label: "渠道台" },
      { href: "/admin", label: "管理台" },
    ],
  },
  {
    title: "账户",
    links: [
      { href: "/login", label: "登录" },
      { href: "/login", label: "注册" },
      { href: "/app", label: "API Key" },
    ],
  },
  {
    title: "状态",
    links: [
      { href: "/", label: "控制面" },
      { href: "/docs", label: "文档" },
      { href: "/admin", label: "审计" },
    ],
  },
];

export function SiteFooter({ brand }: { brand?: Brand }) {
  const name = brand?.name || "TokenHub";
  return (
    <footer className="border-t border-hairline bg-canvas">
      <div className="mx-auto grid max-w-[1200px] gap-10 px-6 py-12 sm:grid-cols-2 lg:grid-cols-4">
        {columns.map((column) => (
          <div key={column.title}>
            <p className="th-eyebrow text-ink-mute">{column.title}</p>
            <ul className="mt-4 flex flex-col gap-2 text-[13px] text-ink-secondary">
              {column.links.map((link) => (
                <li key={`${column.title}-${link.label}`}>
                  <Link href={link.href} className="no-underline hover:text-ink">
                    {link.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <div className="mx-auto flex max-w-[1200px] flex-col gap-2 border-t border-hairline px-6 py-6 text-[13px] text-ink-mute sm:flex-row sm:items-center sm:justify-between">
        <p>
          {name} Clearing · 纸/碳双主题 · 四个入口共用同一套语法
        </p>
        <p>OEM 只换章和 Logo</p>
      </div>
    </footer>
  );
}
