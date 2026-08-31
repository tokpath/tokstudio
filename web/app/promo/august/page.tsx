import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default function AugustPromoPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="promoAugust" primaryHref="/login" secondaryHref="/app" />
      <PublicSection eyebrow="CODE" title="怎么用">
        <div className="rounded-card border border-hairline bg-canvas-raised p-5">
          <p className="font-mono text-lg">THE2E</p>
          <p className="mt-2 text-sm text-ink-secondary">开发兑换码。登录后在首页或用户台兑换，入账单位是 micro-USD。</p>
        </div>
      </PublicSection>
      <Button asChild variant="outline" className="w-fit">
        <Link href="/">返回首页</Link>
      </Button>
    </main>
  );
}
