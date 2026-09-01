import { headers } from "next/headers";
import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Badge } from "@/components/ui/badge";
import { PublicSection, PublicMain } from "@/components/public-section";
import { loadSite } from "@/lib/site-content";
import { IconStamp } from "@/components/icon-stamp";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { AppWindow } from "lucide-react";

export default async function AwesomePage() {
  const t = await getTranslations("awesomeUi");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const apps = site.apps || [];

  return (
    <PublicMain>
      <I18nPublicHero id="awesome" primaryHref="/login" secondaryHref="/vibe-coding" />
      <PublicSection eyebrow="APPS" title={t("appsTitle", { count: apps.length })}>
        <ul className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {apps.map((app) => (
            <li key={app.slug} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <div className="flex items-start justify-between gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone="brand">{app.kind}</Badge>
                  {app.oss ? <span className="th-eyebrow text-success">OSS</span> : null}
                </div>
                <IconStamp icon={AppWindow} size="sm" />
              </div>
              <h3 className="mt-3 text-lg font-semibold">{app.name}</h3>
              <p className="mt-2 text-sm text-ink-secondary">{app.summary}</p>
              <p className="mt-3 text-[12px] text-ink-mute">{app.date}</p>
            </li>
          ))}
        </ul>
      </PublicSection>
      <p className="text-sm text-ink-mute">
        {t("cta")} <Link href="/docs">{t("docs")}</Link> {t("cta2")}
      </p>
    </PublicMain>
  );
}
