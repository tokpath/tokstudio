import type { Metadata } from "next";
import { CodeBlock } from "@/components/code-block";
import { Eyebrow } from "@/components/eyebrow";
import { PriceBookEmpty } from "@/components/price-book";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "模型",
};

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default function ModelsPage() {
  return (
    <PublicShell>
      <div className="flex flex-col gap-8">
        <section className="flex max-w-3xl flex-col gap-4">
          <Eyebrow className="text-brand-emphasis">Price book</Eyebrow>
          <h1 className="text-[40px] font-semibold leading-tight">已发布的模型目录</h1>
          <p className="text-base text-ink-secondary">
            列是 public_model_id、厂商、输入/输出/媒体单价、能力和 Provider 状态字。不展示内部成本价或上游
            Key。目录接通前不编造牌价。
          </p>
        </section>
        <PriceBookEmpty
          title="还没有上架模型"
          detail="公开目录来自已发布价格版本。草稿和内部映射不会出现在这里。"
        />
        <section className="flex max-w-3xl flex-col gap-3">
          <h2 className="text-lg font-semibold">调用示例</h2>
          <p className="text-sm text-ink-secondary">Key 用占位，不写真实完整密钥。</p>
          <CodeBlock>{`curl ${apiBase}/v1/models \\
  -H "Authorization: Bearer sk-...xxxx"`}</CodeBlock>
        </section>
      </div>
    </PublicShell>
  );
}
