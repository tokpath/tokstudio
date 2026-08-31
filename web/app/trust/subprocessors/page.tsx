import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function SubprocessorsPage() {
  const t = await getTranslations("trustUi");
  const rows = [0, 1, 2, 3].map((i) => ({ name: t(`n${i}`), role: t(`r${i}`), region: t(`g${i}`) }));

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="trustSubprocessors" primaryHref="/trust" />
      <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
        <table className="min-w-full text-left text-sm">
          <thead className="border-b border-hairline">
            <tr>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colName")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colRole")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colRegion")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-hairline">
            {rows.map((row) => (
              <tr key={row.name}>
                <td className="px-4 py-3 font-medium">{row.name}</td>
                <td className="px-4 py-3 text-ink-secondary">{row.role}</td>
                <td className="px-4 py-3 text-ink-mute">{row.region}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </main>
  );
}
