import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection, PublicMain } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { IconStamp } from "@/components/icon-stamp";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { TRUST_RETENTION_ICONS, TRUST_SUMMARY_ICONS } from "@/lib/page-icons";

export default async function TrustPage() {
  const t = await getTranslations("trustUi");
  return (
    <PublicMain>
      <I18nPublicHero id="trust" primaryHref="/docs" secondaryHref="/trust/subprocessors" />

      <PublicSection eyebrow="SUMMARY" title={t("sumTitle")}>
        <div className="grid gap-4 md:grid-cols-2">
          {[0, 1, 2, 3].map((i) => (
            <FeatureCard key={i} icon={TRUST_SUMMARY_ICONS[i]} title={t(`s${i}t`)} description={t(`s${i}d`)} />
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="RETENTION" title={t("retTitle")}>
        <ul className="space-y-3 text-sm text-ink-secondary">
          {[0, 1, 2].map((i) => (
            <li key={i} className="flex items-start gap-3.5 rounded-card border border-hairline bg-canvas-raised px-5 py-4">
              <IconStamp icon={TRUST_RETENTION_ICONS[i]} size="sm" className="mt-0.5" />
              <span>
                <strong className="text-ink">{t(`r${i}t`)}</strong>
                {t(`r${i}d`)}
              </span>
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
    </PublicMain>
  );
}
