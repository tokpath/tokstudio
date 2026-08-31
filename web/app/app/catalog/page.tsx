import { headers } from "next/headers";
import { ConsolePageHeader } from "@/components/console/page-header";
import { ModelsCatalog } from "@/components/models-catalog";
import { loadCatalog } from "@/lib/catalog";

export default async function ConsoleCatalogPage({
  searchParams,
}: {
  searchParams: Promise<{ kind?: string; output?: string; q?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const query = await searchParams;
  const initialKind = query.kind || query.output || "all";

  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="CATALOG"
        title="模型广场"
        description={`${models.length} 个模型。搜索、模态筛选、复制模型 ID。价目与公共站同一份目录，控制台用账本密度。`}
      />
      <ModelsCatalog models={models} initialKind={initialKind} />
    </div>
  );
}
