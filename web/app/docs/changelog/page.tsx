import { PublicPageHero, PublicSection } from "@/components/public-section";

const items = [
  { date: "2026-08", title: "公开站按 DESIGN.md 对齐 ofox 页面密度", detail: "模型目录、价目、排行与接入页。" },
  { date: "2026-08", title: "ofox 目录入库", detail: "爬取公开模型 dump 到 catalog，经 /v1/public/models 返回。" },
  { date: "2026-08", title: "四个入口补齐", detail: "公共站 / 用户台 / 渠道台 / 管理台共用纸碳 token。" },
];

export default function ChangelogPage() {
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-10 px-6 py-20">
      <PublicPageHero
        eyebrow="CHANGELOG"
        title="更新日志"
        description="对齐 ofox changelog 入口。只记录本产品已落地的变化。"
        primaryHref="/docs"
        primaryLabel="文档"
      />
      <PublicSection eyebrow="LOG" title="近期">
        <ol className="space-y-3">
          {items.map((item) => (
            <li key={item.title} className="rounded-card border border-hairline bg-canvas-raised p-4">
              <p className="th-eyebrow text-ink-mute">{item.date}</p>
              <p className="mt-1 font-semibold">{item.title}</p>
              <p className="mt-1 text-sm text-ink-secondary">{item.detail}</p>
            </li>
          ))}
        </ol>
      </PublicSection>
    </main>
  );
}
