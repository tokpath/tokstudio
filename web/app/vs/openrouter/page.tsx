import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

const rows = [
  ["公开价目", "已发布价目 + curl", "常见"],
  ["路由可解释", "attempt 回单", "弱"],
  ["渠道 / OEM", "额度与 theme_json 章", "有限"],
  ["账本分列", "客户 / 成本 / 佣金", "视产品"],
  ["主题", "纸/碳/系统", "多为营销深色"],
];

export default function VsOpenRouterPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="vsOpenrouter" primaryHref="/enterprise" secondaryHref="/models" />
      <PublicSection eyebrow="TABLE" title="对照">
        <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
          <table className="min-w-full text-left text-sm">
            <thead className="border-b border-hairline">
              <tr>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">维度</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">TokenHub</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">通用聚合</th>
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
        <Link href="/login">开始使用</Link>
      </Button>
    </main>
  );
}
