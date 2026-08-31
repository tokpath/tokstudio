import Link from "next/link";
import { headers } from "next/headers";
import { fetchAPI } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

const tools = [
  { id: "claude-code", title: "Claude Code", hint: "ANTHROPIC_BASE_URL", env: "export ANTHROPIC_BASE_URL=\"https://{{base}}\"\nexport ANTHROPIC_API_KEY=\"sk-...xxxx\"" },
  { id: "codex", title: "Codex", hint: "~/.codex/config.toml", env: "model_provider = \"tokenhub\"\n\n[model_providers.tokenhub]\nname = \"TokenHub\"\nbase_url = \"https://{{base}}/v1\"\nenv_key = \"TOKENHUB_API_KEY\"" },
  { id: "gemini-cli", title: "Gemini CLI", hint: "自定义 endpoint", env: "export GEMINI_API_BASE=\"https://{{base}}\"\nexport GEMINI_API_KEY=\"sk-...xxxx\"" },
  { id: "opencode", title: "OpenCode", hint: "内置 provider", env: "{\n  \"provider\": {\n    \"tokenhub\": { \"baseURL\": \"https://{{base}}/v1\", \"apiKey\": \"sk-...xxxx\" }\n  }\n}" },
  { id: "cline", title: "Cline", hint: "OpenAI 兼容端点", env: "OpenAI Compatible\nBase URL: https://{{base}}/v1\nAPI Key: sk-...xxxx" },
  { id: "openai-sdk", title: "OpenAI SDK", hint: "base_url", env: "from openai import OpenAI\nclient = OpenAI(base_url=\"https://{{base}}/v1\", api_key=\"sk-...xxxx\")" },
];

export default async function IntegrationsPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let base = "api.tokenhub.local";
  try {
    const docs = await fetchAPI<{ brand?: { api_domain: string } }>("/v1/public/docs-context", { host });
    base = docs.brand?.api_domain || base;
  } catch {
    /* default */
  }

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="docsIntegrations" primaryHref="/quickstart" secondaryHref="/vibe-coding" />
      <PublicSection eyebrow="TOOLS" title="选择你的工具">
        <div className="flex flex-col gap-8">
          {tools.map((tool) => (
            <section key={tool.id} id={tool.id} className="scroll-mt-24">
              <h3 className="text-lg font-semibold">{tool.title}</h3>
              <p className="mb-3 font-mono text-[12px] text-ink-mute">{tool.hint}</p>
              <CodeBlock>{tool.env.replaceAll("{{base}}", base)}</CodeBlock>
            </section>
          ))}
        </div>
      </PublicSection>
      <Button asChild variant="outline" className="w-fit">
        <Link href="/docs">完整文档</Link>
      </Button>
    </main>
  );
}
