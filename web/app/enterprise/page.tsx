import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicPageHero, PublicSection } from "@/components/public-section";

const capabilities = [
  { title: "用量与额度", body: "团队用量可见，渠道额度可管。" },
  { title: "一个 Key", body: "白名单模型共用余额与账本。" },
  { title: "可解释路由", body: "attempt 回单留给财务和值班。" },
  { title: "OEM 换章", body: "域名、Logo、主色可换，语义色不动。" },
  { title: "佣金上限", body: "平台管住底价与佣金天花板。" },
  { title: "审计", body: "高风险操作二次确认并留痕。" },
];

export default function EnterprisePage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-14 px-6 py-16">
      <PublicPageHero
        eyebrow="ENTERPRISE"
        title="企业级 API，团队可控，用量透明"
        description="对齐 ofox 企业页的能力网格与对比叙事；皮肤是清算台所：纸面、细线、钴蓝章。"
        primaryHref="/login"
        primaryLabel="免费开始"
        secondaryHref="/trust"
        secondaryLabel="信任中心"
      />

      <PublicSection eyebrow="CAPABILITIES" title="不止省钱这一件事">
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          {capabilities.map((item) => (
            <div key={item.title} className="rounded-stamp border border-hairline bg-canvas-raised p-5">
              <p className="text-lg font-semibold">{item.title}</p>
              <p className="mt-2 text-sm text-ink-secondary">{item.body}</p>
            </div>
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="COMPARE" title="对照一张表" description="数字与承诺以你们实际上线为准；这里先把结构摆齐。">
        <div className="overflow-x-auto rounded-stamp border border-hairline bg-canvas-raised">
          <table className="min-w-full text-left text-sm">
            <thead className="border-b border-hairline">
              <tr>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">项</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">TokenHub</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">通用聚合网关</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-hairline">
              {[
                ["公开价目", "已发布价目单", "视平台而定"],
                ["路由回单", "attempt 可解释", "常不可见"],
                ["渠道 / OEM", "额度与佣金上限", "弱或无"],
                ["主题", "纸/碳三档", "多锁深色"],
              ].map(([k, a, b]) => (
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

      <section className="rounded-stamp border border-hairline bg-canvas-raised px-6 py-10">
        <h2 className="text-2xl font-semibold">准备好扩展基础设施了吗？</h2>
        <div className="mt-6 flex flex-wrap gap-3">
          <Button asChild>
            <Link href="/login">开始使用</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/vs/openrouter">看完整对比</Link>
          </Button>
        </div>
      </section>
    </main>
  );
}
