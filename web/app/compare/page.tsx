import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { PublicSection, PublicMain } from "@/components/public-section";
import { loadCatalog } from "@/lib/catalog";
import { ModelCompare } from "./compare-client";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function ComparePage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const t = await getTranslations("compareUi");

  return (
    <PublicMain>
      <I18nPublicHero id="compare" primaryHref="/models" secondaryHref="/best-value" />
      <PublicSection eyebrow="TABLE" title={t("table")}>
        <ModelCompare models={models} />
      </PublicSection>
    </PublicMain>
  );
}
