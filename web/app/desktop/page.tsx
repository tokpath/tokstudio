import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicPageHero, PublicSection } from "@/components/public-section";

export default function DesktopPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-12 px-6 py-16">
      <PublicPageHero
        eyebrow="DESKTOP"
        title="Desktop"
        description="对齐 ofox Desktop 入口。客户端分发就绪前，先引导 Web 接入与用户台。"
        primaryHref="/quickstart"
        primaryLabel="Web 快速开始"
        secondaryHref="/login"
        secondaryLabel="登录控制台"
      />
      <PublicSection eyebrow="STATUS" title="当前可用">
        <p className="text-sm text-ink-secondary">
          桌面安装包尚未随本仓库发布。请使用浏览器打开公共站与用户台，或用 Claude Code / Codex 改 base URL。
        </p>
        <Button asChild className="mt-4 w-fit" variant="outline">
          <Link href="/vibe-coding">Vibe Coding</Link>
        </Button>
      </PublicSection>
    </main>
  );
}
