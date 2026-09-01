import Link from "next/link";
import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { fetchAPI } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicSection, StatStrip, PublicMain } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { iconForHref, QUICKSTART_STEP_ICONS } from "@/lib/page-icons";
import { FileCode, Globe, KeyRound, Sparkles } from "lucide-react";

export default async function QuickstartPage() {
  const t = await getTranslations("quickstartUi");
  const th = await getTranslations("home");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let base = "api.tokenhub.local";
  let model = "openai/gpt-5.6-sol";
  try {
    const docs = await fetchAPI<{ brand?: { api_domain: string }; models?: string[] }>("/v1/public/docs-context", {
      host,
    });
    base = docs.brand?.api_domain || base;
    model = docs.models?.[0] || model;
  } catch {
    /* defaults — ofox 快照模型 id 仍可演示 */
  }

  const curl = `curl https://${base}/v1/chat/completions \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model}","messages":[{"role":"user","content":"ping"}]}'`;
  const python = `from openai import OpenAI\n\nclient = OpenAI(\n    base_url="https://${base}/v1",\n    api_key="sk-...xxxx",\n)\n\nr = client.chat.completions.create(\n    model="${model}",\n    messages=[{"role": "user", "content": "ping"}],\n)\nprint(r.choices[0].message.content)`;
  const node = `import OpenAI from "openai";\n\nconst client = new OpenAI({\n  baseURL: "https://${base}/v1",\n  apiKey: "sk-...xxxx",\n});\n\nconst r = await client.chat.completions.create({\n  model: "${model}",\n  messages: [{ role: "user", content: "ping" }],\n});\nconsole.log(r.choices[0].message.content);`;

  return (
    <PublicMain>
      <I18nPublicHero id="quickstart" primaryHref="/login" secondaryHref="/docs" />

      <StatStrip
        items={[
          { label: t("statSteps"), value: "3", hint: t("statStepsHint"), icon: Sparkles },
          { label: t("statProto"), value: t("statProtoValue"), hint: t("statProtoHint"), icon: FileCode },
          { label: "Base URL", value: base, hint: t("statBaseHint"), icon: Globe },
        ]}
      />

      <div className="grid gap-4 lg:grid-cols-3">
        {[
          { n: "1", tk: "s1t", dk: "s1d", ck: "s1c", href: "/login", icon: QUICKSTART_STEP_ICONS[0] },
          { n: "2", tk: "s2t", dk: "s2d", ck: "s2c", href: "#code", icon: QUICKSTART_STEP_ICONS[1] },
          { n: "3", tk: "s3t", dk: "s3d", ck: "s3c", href: "/app", icon: QUICKSTART_STEP_ICONS[2] },
        ].map((s) => (
          <div key={s.n} className="rounded-card border border-hairline bg-canvas-raised p-5">
            <div className="flex items-start justify-between gap-3">
              <p className="th-eyebrow text-brand-emphasis">STEP {s.n}</p>
              <s.icon className="size-4 text-brand-emphasis" strokeWidth={1.75} aria-hidden />
            </div>
            <h3 className="mt-2 text-lg font-semibold">{t(s.tk)}</h3>
            <p className="mt-2 text-sm text-ink-secondary">{t(s.dk)}</p>
            <Button asChild variant="outline" size="sm" className="mt-4">
              <Link href={s.href}>{t(s.ck)}</Link>
            </Button>
          </div>
        ))}
      </div>

      <PublicSection id="code" eyebrow="STEP 2" title={t("codeTitle")} description={t("codeLead")}>
        <div className="flex flex-col gap-6">
          <div>
            <p className="mb-2 text-sm font-medium">cURL</p>
            <CodeBlock>{curl}</CodeBlock>
          </div>
          <div>
            <p className="mb-2 text-sm font-medium">Python</p>
            <CodeBlock>{python}</CodeBlock>
          </div>
          <div>
            <p className="mb-2 text-sm font-medium">Node.js</p>
            <CodeBlock>{node}</CodeBlock>
          </div>
        </div>
      </PublicSection>

      <PublicSection eyebrow="NEXT" title={t("nextTitle")}>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {[
            { href: "/models", label: t("nextModels") },
            { href: "/vibe-coding", label: t("nextVibe") },
            { href: "/docs", label: t("nextDocs") },
            { href: "/enterprise", label: t("nextEnt") },
          ].map((item) => (
            <FeatureCard key={item.href} href={item.href} icon={iconForHref(item.href)} title={item.label} className="p-4" />
          ))}
        </div>
      </PublicSection>

      <section className="rounded-card border border-hairline bg-canvas-raised px-6 py-10 text-center">
        <h2 className="text-2xl font-semibold">{t("readyTitle")}</h2>
        <p className="mt-2 text-sm text-ink-secondary">{t("readyLead")}</p>
        <div className="mt-6 flex flex-wrap justify-center gap-3">
          <Button asChild>
            <Link href="/login">
              <KeyRound />
              {th("ctaKey")}
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/models">{th("ctaModels")}</Link>
          </Button>
        </div>
      </section>
    </PublicMain>
  );
}
