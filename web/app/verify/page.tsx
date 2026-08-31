import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default function VerifyPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="verify" primaryHref="/app" secondaryHref="/docs" />
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
