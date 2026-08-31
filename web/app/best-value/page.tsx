import { headers } from "next/headers";
import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { formatMoney, inferKind, loadCatalog } from "@/lib/catalog";
import { StatStrip } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function BestValuePage() {
  const t = await getTranslations("bestValueUi");
  const tc = await getTranslations("catalog");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const ranked = [...models]
    .filter((m) => inferKind(m) === "text" && Number(m.sell_price?.input) > 0)
    .sort((a, b) => Number(a.sell_price?.input) - Number(b.sell_price?.input))
    .slice(0, 40);

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="bestValue" primaryHref="/login" secondaryHref="/models" />
      <StatStrip
        items={[
          { label: t("statInTable"), value: String(ranked.length) },
          { label: t("statLowest"), value: ranked[0] ? formatMoney(ranked[0].sell_price?.input) : "—" },
          { label: t("statSort"), value: "input ↑", hint: t("statSortHint") },
        ]}
      />
      <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
        <table className="min-w-[880px] w-full text-left text-sm">
          <thead className="border-b border-hairline">
            <tr>
              {["#", tc("colModel"), tc("colVendor"), tc("colInput"), tc("colOutput"), tc("colStatus")].map((h) => (
                <th key={h} className="th-eyebrow px-4 py-3 text-ink-mute">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-hairline">
            {ranked.map((m, i) => (
              <tr key={m.id} className="hover:bg-brand-soft/40">
                <td className="px-4 py-3 font-mono text-ink-mute">{String(i + 1).padStart(2, "0")}</td>
                <td className="px-4 py-3">
                  <Link href={`/models/${m.id}`} className="font-medium no-underline hover:text-brand-emphasis">
                    {m.display_name}
                  </Link>
                  <p className="font-mono text-[11px] text-ink-mute">{m.id}</p>
                </td>
                <td className="px-4 py-3 text-ink-secondary">{m.vendor}</td>
                <td className="px-4 py-3 font-mono tabular-nums text-brand-emphasis">{formatMoney(m.sell_price?.input)}</td>
                <td className="px-4 py-3 font-mono tabular-nums">{formatMoney(m.sell_price?.output)}</td>
                <td className="px-4 py-3 th-eyebrow text-success">AVAILABLE</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </main>
  );
}
