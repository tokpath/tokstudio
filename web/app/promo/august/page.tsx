import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicPageHero, PublicSection } from "@/components/public-section";

export default function AugustPromoPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-12 px-6 py-16">
      <PublicPageHero
        eyebrow="PROMO"
        title="充值优惠"
        description="对齐 ofox 月度充值条：兑换码入账走账本，不做成渐变胶囊。"
        primaryHref="/login"
        primaryLabel="去登录后兑换"
        secondaryHref="/app"
        secondaryLabel="用户台余额"
      />
      <PublicSection eyebrow="CODE" title="怎么用">
        <div className="rounded-stamp border border-hairline bg-canvas-raised p-5">
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
