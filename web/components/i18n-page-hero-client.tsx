"use client";

import { useTranslations } from "next-intl";
import { PublicPageHero } from "@/components/public-section";

export function I18nPublicHeroClient({
  id,
  primaryHref,
  secondaryHref,
  title,
  description,
}: {
  id: string;
  primaryHref: string;
  secondaryHref?: string;
  title?: string;
  description?: string;
}) {
  const t = useTranslations(`public.${id}`);
  return (
    <PublicPageHero
      eyebrow={t("eyebrow")}
      title={title ?? t("title")}
      description={description ?? t("description")}
      primaryHref={primaryHref}
      primaryLabel={t("primary")}
      secondaryHref={secondaryHref}
      secondaryLabel={secondaryHref ? t("secondary") : undefined}
    />
  );
}
