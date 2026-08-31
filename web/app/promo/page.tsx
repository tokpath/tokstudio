import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default function PromoPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="promo" primaryHref="/login" secondaryHref="/channel" />
      <PublicSection eyebrow="HOW" title="怎么合作">
        <ol className="list-decimal space-y-3 pl-5 text-sm text-ink-secondary">
          <li>渠道发放推广码；用户注册时写入归属。</li>
          <li>用量产生佣金冻结 → 可结算，在渠道台查看。</li>
          <li>代理 / KOL 在合作伙伴台看脱敏范围内用户。</li>
        </ol>
      </PublicSection>
      <div className="flex flex-wrap gap-3">
        <Button asChild variant="outline">
          <Link href="/partner">合作伙伴台</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/docs">文档</Link>
        </Button>
      </div>
    </main>
  );
}
