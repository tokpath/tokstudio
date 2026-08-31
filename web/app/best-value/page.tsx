import { headers } from "next/headers";
import Link from "next/link";
import { PublicPageHero, StatStrip } from "@/components/public-section";
import { formatMoney, inferKind, loadCatalog } from "@/lib/catalog";

export default async function BestValuePage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const ranked = [...models]
    .filter((m) => inferKind(m) === "text" && Number(m.sell_price?.input) > 0)
    .sort((a, b) => Number(a.sell_price?.input) - Number(b.sell_price?.input))
    .slice(0, 40);

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <PublicPageHero
        eyebrow="BEST VALUE"
        title="性价比模型"
        description="按公开输入单价从低到高。对齐 ofox 折扣/性价比表的完整密度。"
        primaryHref="/login"
        primaryLabel="获取 API Key"
        secondaryHref="/models"
        secondaryLabel="全部模型"
      />
      <StatStrip
        items={[
          { label: "在表模型", value: String(ranked.length) },
          { label: "最低输入", value: ranked[0] ? formatMoney(ranked[0].sell_price?.input) : "—" },
          { label: "排序", value: "input ↑", hint: "公开卖价" },
        ]}
      />
      <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
        <table className="min-w-[880px] w-full text-left text-sm">
          <thead className="border-b border-hairline">
            <tr>
              {["#", "模型", "厂商", "输入", "输出", "状态"].map((h) => (
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
