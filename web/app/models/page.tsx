import { headers } from "next/headers";
import { ModelsCatalog } from "@/components/models-catalog";
import { PublicPageHero } from "@/components/public-section";
import { loadCatalog } from "@/lib/catalog";

export default async function ModelsPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);

  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-8 px-6 py-12">
      <PublicPageHero
        eyebrow="MODEL CATALOG"
        title="模型目录"
        description={`${models.length} 个模型 · 搜索 / 模态筛选 / 列表与表格。对齐 ofox 模型广场密度，皮肤走 DESIGN.md 价目行。`}
        primaryHref="/login"
        primaryLabel="获取 API Key"
        secondaryHref="/quickstart"
        secondaryLabel="快速开始"
      />
      <ModelsCatalog models={models} />
    </main>
  );
}
