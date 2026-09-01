import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function VsOpenRouterPage() {
  const t = await getTranslations("vsUi");
  const th = await getTranslations("home");
  const rows = [0, 1, 2, 3, 4].map((i) => [t(`r${i}k`), t(`r${i}a`), t(`r${i}b`)]);

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="vsOpenrouter" primaryHref="/enterprise" secondaryHref="/models" />
      <PublicSection eyebrow="TABLE" title={t("tableTitle")}>
        <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
          <table className="min-w-full text-left text-sm">
            <thead className="border-b border-hairline">
              <tr>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colDim")}</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">TokenHub</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colPeer")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-hairline">
              {rows.map(([k, a, b]) => (
                <tr key={k}>
                  <td className="px-4 py-3 font-medium">{k}</td>
                  <td className="px-4 py-3 text-ink-secondary">{a}</td>
                  <td className="px-4 py-3 text-ink-mute">{b}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </PublicSection>
      <Button asChild>
        <Link href="/login">{th("ctaStart")}</Link>
      </Button>
    </main>
  );
}
