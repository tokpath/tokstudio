import { headers } from "next/headers";
import { PublicPageHero, PublicSection } from "@/components/public-section";
import { loadCatalog } from "@/lib/catalog";
import { ModelCompare } from "./compare-client";

export default async function ComparePage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <PublicPageHero
        eyebrow="COMPARE"
        title="模型对比"
        description="对齐 ofox 对照页：选两个已发布模型，并排看价目、上下文和能力。不发明没有的价格，不做橙色营销条。"
        primaryHref="/models"
        primaryLabel="打开价目"
        secondaryHref="/best-value"
        secondaryLabel="性价比"
      />
      <PublicSection eyebrow="TABLE" title="并排价目">
        <ModelCompare models={models} />
      </PublicSection>
    </main>
  );
}
