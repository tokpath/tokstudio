import { PublicPageHero, PublicSection } from "@/components/public-section";

export default function VerifyPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <PublicPageHero
        eyebrow="VERIFY"
        title="模型验真"
        description="对齐 ofox 验真页意图：证明请求打到声明的模型。实现上依赖路由回单与上游映射审计。"
        primaryHref="/app"
        primaryLabel="用户台看回单"
        secondaryHref="/docs"
        secondaryLabel="文档"
      />
      <PublicSection eyebrow="METHOD" title="怎么验">
        <ul className="space-y-3 text-sm text-ink-secondary">
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            每次请求保留 public_model → provider → attempt 与原因码。
          </li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            公开站不展示上游密钥；验真看回单与账本，不靠营销截图。
          </li>
        </ul>
      </PublicSection>
    </main>
  );
}
