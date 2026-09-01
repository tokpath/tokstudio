"use client";

import Link from "next/link";
import { useMemo } from "react";
import { useParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { PublicSection, PublicMain } from "@/components/public-section";
import { I18nPublicHeroClient } from "@/components/i18n-page-hero-client";

const HREFS: Record<string, string> = {
  "best-llm-for-coding": "/models?kind=text",
  "best-llm-for-ai-agents": "/app",
  "best-llm-for-rag": "/models",
  "best-llm-for-vision": "/image",
  "best-llm-for-writing": "/models?kind=text",
  "best-llm-for-data-extraction": "/models",
  "best-llm-for-translation": "/models",
  "best-llm-for-chatbots": "/best-value",
  "best-llm-for-roleplay": "/models?kind=text",
  "cheapest-llm-api": "/best-value",
  "fastest-llm-api": "/quickstart",
  "best-ai-image-generation-model": "/image",
  "best-embedding-model": "/models?kind=embedding",
  "best-llm-for-reasoning": "/models",
  "best-llm-for-long-context": "/models",
};

export default function ModelFinderSlugPage() {
  const params = useParams<{ slug: string }>();
  const t = useTranslations("finderPages");
  const tf = useTranslations("finderUi");
  const tc = useTranslations("common");
  const slug = params.slug;
  const spec = useMemo(() => {
    if (!HREFS[slug]) return null;
    return {
      title: t(`${slug}-t`),
      body: t(`${slug}-d`),
      href: HREFS[slug],
    };
  }, [slug, t]);
  const title = spec?.title || tf("fallbackTitle");
  const body = spec?.body || tf("fallbackBody");
  const href = spec?.href || "/model-finder";

  return (
    <PublicMain>
      <I18nPublicHeroClient id="modelFinder" primaryHref={href} secondaryHref="/model-finder" title={title} />
      <PublicSection eyebrow="WHY" title={tf("whyTitle")}>
        <p className="text-sm text-ink-secondary">{body}</p>
        <Button asChild className="mt-4 w-fit" variant="outline">
          <Link href={href}>{tc("continue")}</Link>
        </Button>
      </PublicSection>
    </PublicMain>
  );
}
