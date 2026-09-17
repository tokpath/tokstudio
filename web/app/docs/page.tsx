import { fetchAPI } from "@/lib/api";
import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { FileCode, MessageSquareText, Terminal, Video } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { CodeBlock } from "@/components/code-block";
import { IconStamp } from "@/components/icon-stamp";
import { PublicMain } from "@/components/public-section";

type DocsContext = {
  brand?: { name: string; api_domain: string };
  models?: string[];
  examples?: { curl: string; python: string; node: string; messages?: string; video?: string };
  notes?: { auth?: string; errors?: string; rate_limit?: string; webhook?: string };
};

const sections = [
  { id: "curl", label: "curl" },
  { id: "python", label: "Python" },
  { id: "node", label: "Node.js" },
  { id: "messages", label: "Messages" },
];

export default async function DocsPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let docs: DocsContext = {};
  try {
    docs = await fetchAPI<DocsContext>("/v1/public/docs-context", { host });
  } catch {
    docs = {};
  }
  const t = await getTranslations("docsUi");
  const tPublic = await getTranslations("public.docs");
  return (
    <PublicMain className="lg:flex-row lg:gap-16">
      <nav className="w-full shrink-0 text-sm text-ink-secondary lg:w-48" aria-label={t("nav")}>
        <p className="th-eyebrow mb-3 text-ink-mute">{tPublic("eyebrow")}</p>
        <ul className="flex flex-row gap-3 overflow-x-auto lg:flex-col lg:gap-2">
          {sections.map((section) => (
            <li key={section.id}>
              <a href={`#${section.id}`} className="inline-flex items-center gap-1.5 whitespace-nowrap no-underline hover:text-ink">
                <FileCode className="size-3.5" strokeWidth={1.75} aria-hidden />
                {section.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
      <article className="flex max-w-[720px] flex-col gap-10">
        <div>
          <Badge tone="brand">{t("badge")}</Badge>
          <h1 className="th-display-sm mt-4">
            {t("title", { name: docs.brand?.name || "TokenHub" })}
          </h1>
          <p className="mt-5 text-base leading-relaxed text-ink-secondary">{t("lead")}</p>
          <p className="mt-5 font-mono text-[13px] text-ink-mute">Base URL · {docs.brand?.api_domain || "api.tokenhub.local"}</p>
          <p className="mt-2 text-sm leading-relaxed text-ink">{t("models", { list: (docs.models || []).join("、") || "—" })}</p>
        </div>
        <section id="curl" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <IconStamp icon={Terminal} size="sm" />
            curl
          </h2>
          <CodeBlock>{docs.examples?.curl || t("curlPh")}</CodeBlock>
        </section>
        <section id="python" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <IconStamp icon={FileCode} size="sm" />
            Python
          </h2>
          <CodeBlock>{docs.examples?.python || t("pythonPh")}</CodeBlock>
        </section>
        <section id="node" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <IconStamp icon={FileCode} size="sm" />
            Node.js
          </h2>
          <CodeBlock>{docs.examples?.node || t("nodePh")}</CodeBlock>
        </section>
        <section id="messages" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <IconStamp icon={MessageSquareText} size="sm" />
            Anthropic Messages
          </h2>
          <CodeBlock>{docs.examples?.messages || t("messagesPh")}</CodeBlock>
        </section>
        {docs.examples?.video ? (
          <section className="flex flex-col gap-3">
            <h2 className="flex items-center gap-2 text-lg font-semibold">
              <IconStamp icon={Video} size="sm" />
              {t("videoTitle")}
            </h2>
            <CodeBlock>{docs.examples.video}</CodeBlock>
          </section>
        ) : null}
        <div className="space-y-3 text-sm leading-relaxed text-ink-secondary">
          <p>{docs.notes?.auth}</p>
          <p>{docs.notes?.errors}</p>
          <p>{docs.notes?.rate_limit}</p>
          <p>{docs.notes?.webhook}</p>
        </div>
      </article>
    </PublicMain>
  );
}
