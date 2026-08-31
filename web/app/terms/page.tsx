import { I18nPublicHero } from "@/components/i18n-page-hero";

export default function TermsPage() {
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-8 px-6 py-20">
      <I18nPublicHero id="terms" primaryHref="/privacy" />
      <article className="space-y-4 text-sm leading-relaxed text-ink-secondary">
        <p>使用本服务即表示你同意按已发布价目计费，并遵守合理使用与安全策略。</p>
        <p>高风险操作（退款、改佣金、改凭据）可能需要二次确认，并写入审计。</p>
        <p>OEM 换皮不得篡改语义色与账本语义。平台保留拒绝不合规 theme_json 的权利。</p>
      </article>
    </main>
  );
}
