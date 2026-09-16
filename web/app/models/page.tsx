import { headers } from "next/headers";
import { ModelsCatalog } from "@/components/models-catalog";
import { loadCatalogPage, parseCatalogSearchParams } from "@/lib/catalog";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { PublicMain } from "@/components/public-section";

export default async function ModelsPage({
  searchParams,
}: {
  searchParams: Promise<{ kind?: string; output?: string; vendor?: string; q?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const query = parseCatalogSearchParams(await searchParams);
  const page = await loadCatalogPage(host, query);

  return (
    <PublicMain>
      <I18nPublicHero id="models" primaryHref="/login" secondaryHref="/quickstart" />
      <ModelsCatalog
        models={page.items}
        facets={page.facets}
        query={query}
        basePath="/models"
        loadOk={page.ok}
        loadMessage={page.message}
      />
    </PublicMain>
  );
}
