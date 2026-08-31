import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function TrustPage() {
  const t = await getTranslations("trustUi");
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-14 px-6 py-20">
      <I18nPublicHero id="trust" primaryHref="/docs" secondaryHref="/trust/subprocessors" />

      <PublicSection eyebrow="SUMMARY" title={t("sumTitle")}>
        <div className="grid gap-3 md:grid-cols-2">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <p className="font-semibold">{t(`s${i}t`)}</p>
              <p className="mt-2 text-sm text-ink-secondary">{t(`s${i}d`)}</p>
            </div>
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="RETENTION" title={t("retTitle")}>
        <ul className="space-y-3 text-sm text-ink-secondary">
          {[0, 1, 2].map((i) => (
            <li key={i} className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
              <strong className="text-ink">{t(`r${i}t`)}</strong>
              {t(`r${i}d`)}
            </li>
          ))}
        </ul>
      </PublicSection>

      <PublicSection eyebrow="LINKS" title={t("linksTitle")}>
        <div className="flex flex-wrap gap-3">
          <Button asChild variant="outline">
            <Link href="/privacy">{t("privacy")}</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/terms">{t("terms")}</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/trust/subprocessors">{t("subs")}</Link>
          </Button>
        </div>
      </PublicSection>
    </main>
  );
}
