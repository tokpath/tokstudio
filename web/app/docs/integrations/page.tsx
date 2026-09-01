import Link from "next/link";
import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { fetchAPI } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicSection, PublicMain } from "@/components/public-section";
import { IconStamp } from "@/components/icon-stamp";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { iconForTool } from "@/lib/page-icons";

export default async function IntegrationsPage() {
  const t = await getTranslations("docsUi");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let base = "api.tokenhub.local";
  try {
    const docs = await fetchAPI<{ brand?: { api_domain: string } }>("/v1/public/docs-context", { host });
    base = docs.brand?.api_domain || base;
  } catch {
    /* default */
  }

  const tools = [
    { id: "claude-code", title: "Claude Code", hint: "ANTHROPIC_BASE_URL", env: "export ANTHROPIC_BASE_URL=\"https://{{base}}\"\nexport ANTHROPIC_API_KEY=\"sk-...xxxx\"" },
    { id: "codex", title: "Codex", hint: "~/.codex/config.toml", env: "model_provider = \"tokenhub\"\n\n[model_providers.tokenhub]\nname = \"TokenHub\"\nbase_url = \"https://{{base}}/v1\"\nenv_key = \"TOKENHUB_API_KEY\"" },
    { id: "gemini-cli", title: "Gemini CLI", hint: t("hintGemini"), env: "export GEMINI_API_BASE=\"https://{{base}}\"\nexport GEMINI_API_KEY=\"sk-...xxxx\"" },
    { id: "opencode", title: "OpenCode", hint: t("hintOpencode"), env: "{\n  \"provider\": {\n    \"tokenhub\": { \"baseURL\": \"https://{{base}}/v1\", \"apiKey\": \"sk-...xxxx\" }\n  }\n}" },
    { id: "cline", title: "Cline", hint: t("hintCline"), env: "OpenAI Compatible\nBase URL: https://{{base}}/v1\nAPI Key: sk-...xxxx" },
    { id: "openai-sdk", title: "OpenAI SDK", hint: "base_url", env: "from openai import OpenAI\nclient = OpenAI(base_url=\"https://{{base}}/v1\", api_key=\"sk-...xxxx\")" },
  ];

  return (
    <PublicMain>
      <I18nPublicHero id="docsIntegrations" primaryHref="/quickstart" secondaryHref="/vibe-coding" />
      <PublicSection eyebrow="TOOLS" title={t("toolsTitle")}>
        <div className="flex flex-col gap-8">
          {tools.map((tool) => (
            <section key={tool.id} id={tool.id} className="scroll-mt-24">
              <h3 className="flex items-center gap-2 text-lg font-semibold">
                <IconStamp icon={iconForTool(tool.title)} size="sm" />
                {tool.title}
              </h3>
              <p className="mb-3 font-mono text-[12px] text-ink-mute">{tool.hint}</p>
              <CodeBlock>{tool.env.replaceAll("{{base}}", base)}</CodeBlock>
            </section>
          ))}
        </div>
      </PublicSection>
      <Button asChild variant="outline" className="w-fit">
        <Link href="/docs">{t("fullDocs")}</Link>
      </Button>
    </PublicMain>
  );
}
