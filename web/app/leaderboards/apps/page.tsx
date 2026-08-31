import { PublicPageHero, PublicSection } from "@/components/public-section";
import { EmptyState } from "@/components/empty-state";

export default function AppsLeaderboardPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-10 px-6 py-16">
      <PublicPageHero
        eyebrow="LEADERBOARD · APPS"
        title="应用 / 工具用量排行"
        description="对齐 ofox 应用榜。无数据时不伪造工具份额。"
        primaryHref="/vibe-coding"
        primaryLabel="编程工具接入"
        secondaryHref="/leaderboards/models"
        secondaryLabel="模型榜"
      />
      <PublicSection eyebrow="TOOLS" title="大家都在用什么客户端？">
        <div className="rounded-stamp border border-hairline bg-canvas-raised">
          <EmptyState title="还没有工具用量汇总" detail="有公开聚合后再上榜。" />
        </div>
      </PublicSection>
    </main>
  );
}
