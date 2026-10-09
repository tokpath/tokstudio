import { getTranslations } from "next-intl/server";
import { PublicMain, PublicPageHero } from "@/components/public-section";
export default async function EnterprisePage() {
  const t = await getTranslations("publicExperience");
  return <PublicMain width="prose"><PublicPageHero eyebrow="OEM" title={t("oemTitle")} description={t("oemDetail")} primaryHref="/enter" primaryLabel={t("oemAction")} secondaryHref="/models" secondaryLabel={t("browseModels")} /></PublicMain>;
}
