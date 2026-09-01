import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function PrivacyPage() {
  const t = await getTranslations("legalUi");
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-8 px-6 py-20">
      <I18nPublicHero id="privacy" primaryHref="/trust" />
      <article className="space-y-4 text-sm leading-relaxed text-ink-secondary">
        <p>{t("p0")}</p>
        <p>{t("p1")}</p>
        <p>{t("p2")}</p>
      </article>
    </main>
  );
}
