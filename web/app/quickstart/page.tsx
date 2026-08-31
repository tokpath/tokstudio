import Link from "next/link";
import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { fetchAPI } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicSection, StatStrip } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

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
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-14 px-6 py-20">
      <I18nPublicHero id="quickstart" primaryHref="/login" secondaryHref="/docs" />

      <StatStrip
        items={[
          { label: t("statSteps"), value: "3", hint: t("statStepsHint") },
          { label: t("statProto"), value: t("statProtoValue"), hint: t("statProtoHint") },
          { label: "Base URL", value: base, hint: t("statBaseHint") },
        ]}
      />

      <div className="grid gap-4 lg:grid-cols-3">
        {[
          { n: "1", tk: "s1t", dk: "s1d", ck: "s1c", href: "/login" },
          { n: "2", tk: "s2t", dk: "s2d", ck: "s2c", href: "#code" },
          { n: "3", tk: "s3t", dk: "s3d", ck: "s3c", href: "/app" },
        ].map((s) => (
          <div key={s.n} className="rounded-card border border-hairline bg-canvas-raised p-5">
            <p className="th-eyebrow text-brand-emphasis">STEP {s.n}</p>
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
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {[
            { href: "/models", label: t("nextModels") },
            { href: "/vibe-coding", label: t("nextVibe") },
            { href: "/docs", label: t("nextDocs") },
            { href: "/enterprise", label: t("nextEnt") },
          ].map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="rounded-card border border-hairline bg-canvas-raised px-4 py-3 text-sm no-underline hover:bg-brand-soft/40"
            >
              {item.label}
            </Link>
          ))}
        </div>
      </PublicSection>

      <section className="rounded-card border border-hairline bg-canvas-raised px-6 py-10 text-center">
        <h2 className="text-2xl font-semibold">{t("readyTitle")}</h2>
        <p className="mt-2 text-sm text-ink-secondary">{t("readyLead")}</p>
        <div className="mt-6 flex flex-wrap justify-center gap-3">
          <Button asChild>
            <Link href="/login">{th("ctaKey")}</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/models">{th("ctaModels")}</Link>
          </Button>
        </div>
      </section>
    </main>
  );
}
