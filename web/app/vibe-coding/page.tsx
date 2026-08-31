import Link from "next/link";
import { headers } from "next/headers";
import { fetchAPI } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicPageHero, PublicSection, StatStrip } from "@/components/public-section";

export default async function VibeCodingPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let base = "api.tokenhub.local";
  try {
    const docs = await fetchAPI<{ brand?: { api_domain: string } }>("/v1/public/docs-context", { host });
    base = docs.brand?.api_domain || base;
  } catch {
    /* default */
  }

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-14 px-6 py-20">
      <PublicPageHero
        eyebrow="VIBE CODING"
        title="编程工具，只需换一个 URL"
        description="对齐 ofox Vibe Coding：Codex / Claude Code 配置片段落在碳面代码块。"
        primaryHref="/login"
        primaryLabel="获取 API Key"
        secondaryHref="/models"
        secondaryLabel="探索模型"
      />

      <StatStrip
        items={[
          { label: "协议", value: "原生兼容", hint: "OpenAI / Anthropic" },
          { label: "改动", value: "base URL", hint: "不动 SDK" },
          { label: "下一步", value: "用户台", hint: "建 Key 看用量" },
        ]}
      />

      <PublicSection eyebrow="CODEX" title="~/.codex/config.toml">
        <CodeBlock>{`model_provider = "tokenhub"\n\n[model_providers.tokenhub]\nname = "TokenHub"\nbase_url = "https://${base}/v1"\nenv_key = "TOKENHUB_API_KEY"`}</CodeBlock>
      </PublicSection>

      <PublicSection eyebrow="CLAUDE CODE" title="环境变量">
        <CodeBlock>{`export ANTHROPIC_BASE_URL="https://${base}"\nexport ANTHROPIC_API_KEY="sk-...xxxx"`}</CodeBlock>
      </PublicSection>

      <div className="flex flex-wrap gap-3">
        <Button asChild>
          <Link href="/quickstart">快速开始</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/docs">文档</Link>
        </Button>
      </div>
    </main>
  );
}
