import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection, PublicMain } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { ENTERPRISE_CAP_ICONS } from "@/lib/page-icons";

export default async function EnterprisePage() {
  const t = await getTranslations("enterpriseUi");
  const th = await getTranslations("home");
  const capabilities = [0, 1, 2, 3, 4, 5].map((i) => ({ title: t(`c${i}t`), body: t(`c${i}d`) }));
  const rows = [0, 1, 2, 3].map((i) => [t(`r${i}k`), t(`r${i}a`), t(`r${i}b`)]);

  return (
    <PublicMain>
      <I18nPublicHero id="enterprise" primaryHref="/login" secondaryHref="/trust" />

      <PublicSection eyebrow="CAPABILITIES" title={t("capsTitle")}>
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {capabilities.map((item, i) => (
            <FeatureCard key={item.title} icon={ENTERPRISE_CAP_ICONS[i]} title={item.title} description={item.body} />
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="COMPARE" title={t("tableTitle")} description={t("tableLead")}>
        <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
          <table className="min-w-full text-left text-sm">
            <thead className="border-b border-hairline">
              <tr>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colItem")}</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">TokenHub</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colPeer")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-hairline">
              {rows.map(([k, a, b]) => (
                <tr key={k}>
                  <td className="px-4 py-3 text-ink">{k}</td>
                  <td className="px-4 py-3 text-ink-secondary">{a}</td>
                  <td className="px-4 py-3 text-ink-mute">{b}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </PublicSection>

      <section className="rounded-card border border-hairline bg-canvas-raised px-6 py-10">
        <h2 className="text-2xl font-semibold">{t("readyTitle")}</h2>
        <div className="mt-6 flex flex-wrap gap-3">
          <Button asChild>
            <Link href="/login">{th("ctaStart")}</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/vs/openrouter">{t("fullCompare")}</Link>
          </Button>
        </div>
      </section>
    </PublicMain>
  );
}
