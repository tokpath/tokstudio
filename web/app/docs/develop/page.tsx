import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default function DocsDevelopPage() {
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="docsDevelop" primaryHref="/docs" secondaryHref="/trust" />
      <PublicSection eyebrow="ERRORS" title="产品文案，不要只画红框">
        <ul className="space-y-3 text-sm text-ink-secondary">
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <span className="font-mono text-danger">402</span> 余额不足，先充值或兑换。
          </li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <span className="font-mono text-hold">429</span> 限流，回单会记 attempt 与 fallback。
          </li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <span className="font-mono text-danger">403</span> 模型不在 Key 白名单，或 Key 已禁用。
          </li>
        </ul>
      </PublicSection>
      <PublicSection eyebrow="SECURITY" title="数据怎么走">
        <p className="text-sm text-ink-secondary">
          同步 API 不落业务正文。媒体任务按生命周期短时保留。完整 Key 默认掩码。
        </p>
      </PublicSection>
    </main>
  );
}
