import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicPageHero, PublicSection } from "@/components/public-section";

const rows = [
  ["公开价目", "已发布价目 + curl", "常见"],
  ["路由可解释", "attempt 回单", "弱"],
  ["渠道 / OEM", "额度与 theme_json 章", "有限"],
  ["账本分列", "客户 / 成本 / 佣金", "视产品"],
  ["主题", "纸/碳/系统", "多为营销深色"],
];

export default function VsOpenRouterPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-12 px-6 py-16">
      <PublicPageHero
        eyebrow="COMPARE"
        title="TokenHub vs 通用聚合网关"
        description="对齐 ofox「vs OpenRouter」对照页结构；用清算台能力说话，不做橙色营销条。"
        primaryHref="/enterprise"
        primaryLabel="企业能力"
        secondaryHref="/models"
        secondaryLabel="看价目"
      />
      <PublicSection eyebrow="TABLE" title="对照">
        <div className="overflow-x-auto rounded-stamp border border-hairline bg-canvas-raised">
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
