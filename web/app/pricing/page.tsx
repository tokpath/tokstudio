import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { CircleDollarSign, FileText, KeyRound, Scale } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PublicSection, StatStrip, PublicMain } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { PRICING_KIND_ICONS } from "@/lib/page-icons";

export default async function PricingPage() {
  const t = await getTranslations("pricingUi");
  const th = await getTranslations("home");
  return (
    <PublicMain>
      <I18nPublicHero id="pricing" primaryHref="/models" secondaryHref="/best-value" />
      <StatStrip
        items={[
          { label: t("statFee"), value: "0%", hint: t("statFeeHint"), icon: CircleDollarSign },
          { label: t("statBill"), value: t("statBillValue"), hint: t("statBillHint"), icon: Scale },
          { label: t("statSettle"), value: t("statSettleValue"), hint: t("statSettleHint"), icon: FileText },
        ]}
      />
      <PublicSection eyebrow="HOW" title={t("howTitle")}>
        <div className="grid gap-4 md:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <FeatureCard key={i} icon={PRICING_KIND_ICONS[i]} title={t(`t${i}`)} description={t(`d${i}`)} />
          ))}
        </div>
      </PublicSection>
      <Button asChild>
        <Link href="/login">
          <KeyRound />
          {th("ctaKey")}
        </Link>
      </Button>
    </PublicMain>
  );
}
