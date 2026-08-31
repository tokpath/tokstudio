import { headers } from "next/headers";
import { ModelsCatalog } from "@/components/models-catalog";
import { loadCatalog } from "@/lib/catalog";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function ModelsPage({
  searchParams,
}: {
  searchParams: Promise<{ kind?: string; output?: string; q?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const query = await searchParams;
  const initialKind = query.kind || query.output || "all";

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 py-20">
      <I18nPublicHero id="models" primaryHref="/login" secondaryHref="/quickstart" />
      <ModelsCatalog models={models} initialKind={initialKind} />
    </main>
  );
}
