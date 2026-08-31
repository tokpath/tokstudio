import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicPageHero, PublicSection } from "@/components/public-section";

export default function TrustPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-14 px-6 py-20">
      <PublicPageHero
        eyebrow="TRUST CENTER"
        title="可以放心构建的安全基础"
        description="对齐 ofox 信任中心：留存边界、加密、Key、可用性。文案按 TokenHub 清算台口径。"
        primaryHref="/docs"
        primaryLabel="安全相关文档"
        secondaryHref="/trust/subprocessors"
        secondaryLabel="第三方服务商"
      />

      <PublicSection eyebrow="SUMMARY" title="四点摘要">
        <div className="grid gap-3 md:grid-cols-2">
          {[
            { t: "同步 API 不落业务正文", d: "网关只做转发与计费所需元数据。" },
            { t: "传输加密", d: "客户到平台、平台到上游使用 TLS。" },
            { t: "API Key 保护", d: "完整 Key 默认掩码，轮换与禁用走审计。" },
            { t: "控制面可见", d: "公共站与管理台都能看就绪态字标。" },
          ].map((item) => (
            <div key={item.t} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <p className="font-semibold">{item.t}</p>
              <p className="mt-2 text-sm text-ink-secondary">{item.d}</p>
            </div>
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="RETENTION" title="会保存什么">
        <ul className="space-y-3 text-sm text-ink-secondary">
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <strong className="text-ink">标准模型 API：</strong>不持久化 prompt/completion 正文；保留计费与路由回单字段。
          </li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <strong className="text-ink">媒体异步任务：</strong>任务状态与必要交付物短时保留，到期清理。
          </li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <strong className="text-ink">账本：</strong>客户收费、上游成本、佣金分列，不可改写已结算行。
          </li>
        </ul>
      </PublicSection>

      <PublicSection eyebrow="LINKS" title="信任与合规资料">
        <div className="flex flex-wrap gap-3">
          <Button asChild variant="outline">
            <Link href="/privacy">隐私政策</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/terms">服务条款</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/trust/subprocessors">子处理商</Link>
          </Button>
        </div>
      </PublicSection>
    </main>
  );
}
