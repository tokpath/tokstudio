import { PublicPageHero } from "@/components/public-section";

export default function PrivacyPage() {
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-8 px-6 py-16">
      <PublicPageHero
        eyebrow="PRIVACY"
        title="隐私政策"
        description="对齐 ofox 隐私页入口。细则与信任中心一致：同步 API 不落业务正文，账本与路由元数据保留。"
        primaryHref="/trust"
        primaryLabel="信任中心"
      />
      <article className="space-y-4 text-sm leading-relaxed text-ink-secondary">
        <p>我们处理账户邮箱、计费与用量元数据，以便提供服务、对账与安全防护。</p>
        <p>标准同步推理请求的正文默认不作为业务库长期存储；媒体任务按任务生命周期短时保留。</p>
        <p>子处理商列表见信任中心。你可通过账户设置管理基础资料（以产品实际能力为准）。</p>
      </article>
    </main>
  );
}
