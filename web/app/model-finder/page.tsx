"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

const SCENE_IDS = ["coding", "agent", "rag", "vision", "cheap", "fast"] as const;

export default function ModelFinderPage() {
  const t = useTranslations("finderUi");
  const tc = useTranslations("common");
  const [scene, setScene] = useState<(typeof SCENE_IDS)[number]>("coding");
  const recs = useMemo(() => {
    const pairs: Record<(typeof SCENE_IDS)[number], [string, string][]> = {
      coding: [
        ["coding0t", "coding0d"],
        ["coding1t", "coding1d"],
      ],
      agent: [
        ["agent0t", "agent0d"],
        ["agent1t", "agent1d"],
      ],
      rag: [
        ["rag0t", "rag0d"],
        ["rag1t", "rag1d"],
      ],
      vision: [
        ["vision0t", "vision0d"],
        ["vision1t", "vision1d"],
      ],
      cheap: [["cheap0t", "cheap0d"]],
      fast: [["fast0t", "fast0d"]],
    };
    const hrefs: Record<string, string> = {
      coding0t: "/models",
      coding1t: "/vibe-coding",
      agent0t: "/app",
      agent1t: "/quickstart",
      rag0t: "/models",
      rag1t: "/best-value",
      vision0t: "/image",
      vision1t: "/models",
      cheap0t: "/best-value",
      fast0t: "/quickstart",
    };
    return pairs[scene].map(([titleKey, whyKey]) => ({
      title: t(titleKey),
      why: t(whyKey),
      href: hrefs[titleKey],
    }));
  }, [scene, t]);

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="modelFinder" primaryHref="/login" secondaryHref="/models" />

      <PublicSection eyebrow="SCENE" title={t("sceneTitle")}>
        <div className="flex flex-wrap gap-2">
          {SCENE_IDS.map((id) => (
            <button
              key={id}
              type="button"
              onClick={() => setScene(id)}
              className={`rounded-control px-3 py-2 text-sm ${
                scene === id ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-secondary"
              }`}
            >
              {t(id)}
            </button>
          ))}
        </div>
        <p className="text-sm text-ink-mute">{t(`${scene}Hint`)}</p>
      </PublicSection>

      <PublicSection eyebrow="RESULT" title={t("resultTitle")}>
        <ul className="space-y-3">
          {recs.map((r) => (
            <li key={r.href + r.title} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <p className="font-semibold">{r.title}</p>
              <p className="mt-1 text-sm text-ink-secondary">{r.why}</p>
              <Button asChild variant="outline" size="sm" className="mt-3">
                <Link href={r.href}>{tc("open")}</Link>
              </Button>
            </li>
          ))}
        </ul>
      </PublicSection>
    </main>
  );
}
