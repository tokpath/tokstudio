import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection, StatStrip } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function PricingPage() {
  const t = await getTranslations("pricingUi");
  const th = await getTranslations("home");
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-14 px-6 py-20">
      <I18nPublicHero id="pricing" primaryHref="/models" secondaryHref="/best-value" />
      <StatStrip
        items={[
          { label: t("statFee"), value: "0%", hint: t("statFeeHint") },
          { label: t("statBill"), value: t("statBillValue"), hint: t("statBillHint") },
          { label: t("statSettle"), value: t("statSettleValue"), hint: t("statSettleHint") },
        ]}
      />
      <PublicSection eyebrow="HOW" title={t("howTitle")}>
        <div className="grid gap-3 md:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <p className="font-semibold">{t(`t${i}`)}</p>
              <p className="mt-2 text-sm text-ink-secondary">{t(`d${i}`)}</p>
            </div>
          ))}
        </div>
      </PublicSection>
      <Button asChild>
        <Link href="/login">{th("ctaKey")}</Link>
      </Button>
    </main>
  );
}
