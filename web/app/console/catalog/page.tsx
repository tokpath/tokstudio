import { headers } from "next/headers";
import { ModelsCatalog } from "@/components/models-catalog";
import { loadCatalog } from "@/lib/catalog";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

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
      <I18nConsoleHeader id="catalog" />
      <ModelsCatalog models={models} initialKind={initialKind} />
    </div>
  );
}
