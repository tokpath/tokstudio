import type { Metadata } from "next";
import { CodeBlock } from "@/components/code-block";
import { Eyebrow } from "@/components/eyebrow";
import { PriceBookEmpty } from "@/components/price-book";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "定价",
};

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default function PricingPage() {
  return (
    <PublicShell>
      <div className="flex flex-col gap-8">
        <section className="flex max-w-3xl flex-col gap-4">
          <Eyebrow className="text-brand-emphasis">Published book</Eyebrow>
          <h1 className="text-[40px] font-semibold leading-tight">一张已发布的价目单</h1>
          <p className="text-base text-ink-secondary">
            新价格只影响新请求。历史账单保持当时快照，不会因为改价而变色闪烁。金额用等宽数字，状态带字。
          </p>
        </section>
        <PriceBookEmpty
          title="还没有已发布价格"
          detail="价目来自已发布版本。这里不会预填示例单价，避免被当成真牌价。"
        />
        <section className="flex max-w-3xl flex-col gap-3">
          <h2 className="text-lg font-semibold">探活</h2>
          <CodeBlock>{`curl ${apiBase}/healthz`}</CodeBlock>
        </section>
      </div>
    </PublicShell>
  );
}
