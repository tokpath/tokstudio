import { PublicPageHero } from "@/components/public-section";

const rows = [
  { name: "PostgreSQL", role: "主库与账本", region: "按部署" },
  { name: "Redis", role: "会话 / 限流 / 队列辅助", region: "按部署" },
  { name: "上游模型提供商", role: "推理与媒体生成", region: "按路由" },
  { name: "支付适配器", role: "充值与结算", region: "按配置" },
];

export default function SubprocessorsPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-10 px-6 py-16">
      <PublicPageHero
        eyebrow="SUBPROCESSORS"
        title="第三方服务商"
        description="基础设施、支付与条件性模型上游。对齐 ofox 子处理商列表结构。"
        primaryHref="/trust"
        primaryLabel="返回信任中心"
      />
      <div className="overflow-x-auto rounded-stamp border border-hairline bg-canvas-raised">
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
