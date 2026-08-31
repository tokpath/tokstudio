import { PublicPageHero } from "@/components/public-section";

export default function TermsPage() {
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-8 px-6 py-16">
      <PublicPageHero
        eyebrow="TERMS"
        title="服务条款"
        description="占位条款页，结构对齐 ofox 法律页。正式法务文案需由法务审定后替换。"
        primaryHref="/privacy"
        primaryLabel="隐私政策"
      />
      <article className="space-y-4 text-sm leading-relaxed text-ink-secondary">
        <p>使用本服务即表示你同意按已发布价目计费，并遵守合理使用与安全策略。</p>
        <p>高风险操作（退款、改佣金、改凭据）可能需要二次确认，并写入审计。</p>
        <p>OEM 换皮不得篡改语义色与账本语义。平台保留拒绝不合规 theme_json 的权利。</p>
      </article>
    </main>
  );
}
