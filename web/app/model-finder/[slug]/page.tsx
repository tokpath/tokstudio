"use client";

import Link from "next/link";
import { useMemo } from "react";
import { useParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import { PublicPageHero, PublicSection } from "@/components/public-section";

const PAGES: Record<string, { title: string; body: string; href: string }> = {
  "best-llm-for-coding": { title: "编程该用哪个模型", body: "优先 tools / reasoning，再看输入单价。", href: "/models?kind=text" },
  "best-llm-for-ai-agents": { title: "Agent 该用哪个模型", body: "长上下文 + 函数调用，失败看路由回单。", href: "/app" },
  "best-llm-for-rag": { title: "RAG / 长文档", body: "看 context_length 与输入成本。", href: "/models" },
  "best-llm-for-vision": { title: "视觉与图像", body: "理解走带 vision 的文本模型，生成走图像模型。", href: "/image" },
  "best-llm-for-writing": { title: "写作", body: "按价目挑长输出友好的文本模型。", href: "/models?kind=text" },
  "best-llm-for-data-extraction": { title: "抽取 / JSON", body: "选支持 structured / json 的模型。", href: "/models" },
  "best-llm-for-translation": { title: "翻译", body: "多语回退字体已在 DESIGN.md；模型看价目。", href: "/models" },
  "best-llm-for-chatbots": { title: "对话机器人", body: "延迟与单价平衡，从 flash 档试起。", href: "/best-value" },
  "best-llm-for-roleplay": { title: "角色扮演", body: "长上下文文本模型，注意合规。", href: "/models?kind=text" },
  "cheapest-llm-api": { title: "最便宜的 API", body: "按公开输入单价排序。", href: "/best-value" },
  "fastest-llm-api": { title: "更快的档", body: "flash / mini / turbo 类，先跑通再换旗舰。", href: "/quickstart" },
  "best-ai-image-generation-model": { title: "图像生成", body: "按张计价，细节在模型页。", href: "/image" },
  "best-embedding-model": { title: "向量模型", body: "目录里筛 embedding。", href: "/models?kind=embedding" },
  "best-llm-for-reasoning": { title: "推理", body: "带 reasoning 标签的价目行。", href: "/models" },
  "best-llm-for-long-context": { title: "长上下文", body: "看 context_length 列，金额等宽。", href: "/models" },
};

export default function ModelFinderSlugPage() {
  const params = useParams<{ slug: string }>();
  const spec = useMemo(() => PAGES[params.slug] || null, [params.slug]);
  const title = spec?.title || "模型推荐";
  const body = spec?.body || "回到推荐器勾选场景，或直接打开价目。";
  const href = spec?.href || "/model-finder";

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <PublicPageHero
        eyebrow="MODEL FINDER"
        title={title}
        description={`对齐 ofox /model-finder/${params.slug} 落地页。推荐链到本站价目与接入，不伪造跑分。`}
        primaryHref={href}
        primaryLabel="打开对应目录"
        secondaryHref="/model-finder"
        secondaryLabel="全部场景"
      />
      <PublicSection eyebrow="WHY" title="怎么选">
        <p className="text-sm text-ink-secondary">{body}</p>
        <Button asChild className="mt-4 w-fit" variant="outline">
          <Link href={href}>继续</Link>
        </Button>
      </PublicSection>
    </main>
  );
}
