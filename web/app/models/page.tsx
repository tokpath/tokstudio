import { headers } from "next/headers";
import { ModelsCatalog } from "@/components/models-catalog";
import { loadCatalogPage, parseCatalogSearchParams } from "@/lib/catalog";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function ModelsPage({
  searchParams,
}: {
  searchParams: Promise<{ kind?: string; output?: string; vendor?: string; q?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const query = parseCatalogSearchParams(await searchParams);
  const page = await loadCatalogPage(host, query);

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 py-20">
      <I18nPublicHero id="models" primaryHref="/login" secondaryHref="/quickstart" />
      <ModelsCatalog models={page.items} facets={page.facets} query={query} basePath="/models" />
    </main>
  );
}
