import { headers } from "next/headers";
import { PublicPageHero, PublicSection } from "@/components/public-section";
import { LeaderboardTable } from "@/components/leaderboard-table";
import { loadSite } from "@/lib/site-content";

export default async function ModelsLeaderboardPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const rows = site.leaderboards?.models || [];

  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-10 px-6 py-16">
      <PublicPageHero
        eyebrow="LEADERBOARD · MODELS"
        title="模型用量排行"
        description={`${site.leaderboards?.window || "公开用量窗口"}。份额来自 ofox 公开榜快照，经服务端 /v1/public/site 返回。`}
        primaryHref="/models"
        primaryLabel="去价目"
        secondaryHref="/leaderboards/apps"
        secondaryLabel="应用榜"
      />
      <PublicSection eyebrow="SHARE" title="过去周期 Token 份额">
        <LeaderboardTable rows={rows} />
      </PublicSection>
      <p className="text-sm text-ink-mute">{site.leaderboards?.note}</p>
    </main>
  );
}
