import { headers } from "next/headers";
import { ModelsCatalog } from "@/components/models-catalog";
import { loadCatalogPage, parseCatalogSearchParams } from "@/lib/catalog";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default async function ConsoleCatalogPage({
  searchParams,
}: {
  searchParams: Promise<{ kind?: string; output?: string; vendor?: string; q?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const query = parseCatalogSearchParams(await searchParams);
  const page = await loadCatalogPage(host, query);

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="catalog" />
      <ModelsCatalog
        models={page.items}
        facets={page.facets}
        query={query}
        basePath="/app/catalog"
        loadOk={page.ok}
        loadMessage={page.message}
      />
    </div>
  );
}
