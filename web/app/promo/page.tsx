import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicPageHero, PublicSection } from "@/components/public-section";

export default function PromoPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-12 px-6 py-16">
      <PublicPageHero
        eyebrow="AFFILIATE"
        title="合作推广"
        description="对齐 ofox 合作页：介绍推广码与渠道结算入口。注册时推广码由服务端固化，不能自助改归属。"
        primaryHref="/login"
        primaryLabel="注册并填写推广码"
        secondaryHref="/channel"
        secondaryLabel="渠道台"
      />
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
