import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicSection, StatStrip } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default function PricingPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-14 px-6 py-20">
      <I18nPublicHero id="pricing" primaryHref="/models" secondaryHref="/best-value" />
      <StatStrip
        items={[
          { label: "平台费", value: "0%", hint: "不抽成标价" },
          { label: "计费", value: "按量", hint: "token / 秒 / 张" },
          { label: "结算", value: "账本", hint: "客户账可复算" },
        ]}
      />
      <PublicSection eyebrow="HOW" title="怎么计">
        <div className="grid gap-3 md:grid-cols-3">
          {[
            { t: "文本", d: "输入 / 输出按百万 token，等宽数字。" },
            { t: "图像", d: "按张或输出图像单价。" },
            { t: "视频", d: "按秒，分辨率出现在模型详情。" },
          ].map((item) => (
            <div key={item.t} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <p className="font-semibold">{item.t}</p>
              <p className="mt-2 text-sm text-ink-secondary">{item.d}</p>
            </div>
          ))}
        </div>
      </PublicSection>
      <Button asChild>
        <Link href="/login">获取 API Key</Link>
      </Button>
    </main>
  );
}
