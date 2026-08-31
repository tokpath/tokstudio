import { I18nPublicHero } from "@/components/i18n-page-hero";

const rows = [
  { name: "PostgreSQL", role: "主库与账本", region: "按部署" },
  { name: "Redis", role: "会话 / 限流 / 队列辅助", region: "按部署" },
  { name: "上游模型提供商", role: "推理与媒体生成", region: "按路由" },
  { name: "支付适配器", role: "充值与结算", region: "按配置" },
];

export default function SubprocessorsPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="trustSubprocessors" primaryHref="/trust" />
      <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
        <table className="min-w-full text-left text-sm">
          <thead className="border-b border-hairline">
            <tr>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">名称</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">用途</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">区域</th>
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
