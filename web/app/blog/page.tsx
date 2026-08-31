import Link from "next/link";
import { PublicPageHero } from "@/components/public-section";
import { EmptyState } from "@/components/empty-state";

export default function BlogPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-10 px-6 py-16">
      <PublicPageHero
        eyebrow="BLOG"
        title="博客"
        description="对齐 ofox 博客列表壳。文章源接入前保持空状态，不塞假文。"
        primaryHref="/docs"
        primaryLabel="先看文档"
        secondaryHref="/quickstart"
        secondaryLabel="快速开始"
      />
      <div className="rounded-stamp border border-hairline bg-canvas-raised">
        <EmptyState title="还没有文章" detail="CMS 或 MDX 源接上后在此列出。" />
      </div>
      <p className="text-sm text-ink-mute">
        需要接入说明？去 <Link href="/docs">文档</Link>。
      </p>
    </main>
  );
}
