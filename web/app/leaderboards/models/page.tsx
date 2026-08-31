import Link from "next/link";
import { PublicPageHero, PublicSection } from "@/components/public-section";
import { EmptyState } from "@/components/empty-state";

export default function ModelsLeaderboardPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-10 px-6 py-16">
      <PublicPageHero
        eyebrow="LEADERBOARD · MODELS"
        title="模型用量排行"
        description="对齐 ofox 模型榜结构。真实份额要等用量事件汇聚；未接通时用空状态，不画假曲线。"
        primaryHref="/models"
        primaryLabel="去价目"
        secondaryHref="/leaderboards/apps"
        secondaryLabel="应用榜"
      />
      <PublicSection eyebrow="SHARE" title="过去周期 Token 份额">
        <div className="rounded-stamp border border-hairline bg-canvas-raised">
          <EmptyState title="还没有公开用量份额" detail="接通用量汇总后，这里显示与公开榜同一组数据。" />
        </div>
      </PublicSection>
      <p className="text-sm text-ink-mute">
        也可先浏览 <Link href="/best-value">性价比价目</Link>。
      </p>
    </main>
  );
}
