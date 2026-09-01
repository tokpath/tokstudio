import Link from "next/link";
import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { fetchAPI } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicSection, StatStrip, PublicMain } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { Globe, Sparkles, Terminal } from "lucide-react";

export default async function VibeCodingPage() {
  const t = await getTranslations("vibeUi");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let base = "api.tokenhub.local";
  try {
    const docs = await fetchAPI<{ brand?: { api_domain: string } }>("/v1/public/docs-context", { host });
    base = docs.brand?.api_domain || base;
  } catch {
    /* default */
  }

  return (
    <PublicMain>
      <I18nPublicHero id="vibeCoding" primaryHref="/login" secondaryHref="/models" />

      <StatStrip
        items={[
          { label: t("statProto"), value: t("statProtoValue"), hint: "OpenAI / Anthropic", icon: Sparkles },
          { label: t("statChange"), value: "base URL", hint: t("statChangeHint"), icon: Globe },
          { label: t("statNext"), value: t("statNextValue"), hint: t("statNextHint"), icon: Terminal },
        ]}
      />

      <PublicSection eyebrow="CODEX" title="~/.codex/config.toml">
        <CodeBlock>{`model_provider = "tokenhub"\n\n[model_providers.tokenhub]\nname = "TokenHub"\nbase_url = "https://${base}/v1"\nenv_key = "TOKENHUB_API_KEY"`}</CodeBlock>
      </PublicSection>

      <PublicSection eyebrow="CLAUDE CODE" title={t("envTitle")}>
        <CodeBlock>{`export ANTHROPIC_BASE_URL="https://${base}"\nexport ANTHROPIC_API_KEY="sk-...xxxx"`}</CodeBlock>
      </PublicSection>

      <div className="flex flex-wrap gap-3">
        <Button asChild>
          <Link href="/quickstart">{t("qs")}</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/docs">{t("docs")}</Link>
        </Button>
      </div>
    </PublicMain>
  );
}
