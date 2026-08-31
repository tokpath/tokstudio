import { fetchAPI } from "@/lib/api";
import { headers } from "next/headers";
import { Badge } from "@/components/ui/badge";
import { CodeBlock } from "@/components/code-block";

type DocsContext = {
  brand?: { name: string; api_domain: string };
  models?: string[];
  examples?: { curl: string; python: string; node: string; messages?: string; video?: string };
  notes?: { errors?: string; rate_limit?: string; webhook?: string };
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
  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-10 px-6 py-16 lg:flex-row">
      <nav className="w-full shrink-0 text-sm text-ink-secondary lg:w-48" aria-label="文档目录">
        <p className="th-eyebrow mb-3 text-ink-mute">Docs</p>
        <ul className="flex flex-row gap-3 overflow-x-auto lg:flex-col lg:gap-2">
          {sections.map((section) => (
            <li key={section.id}>
              <a href={`#${section.id}`} className="whitespace-nowrap no-underline hover:text-ink">
                {section.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
      <article className="flex max-w-[720px] flex-col gap-10">
        <div>
          <Badge tone="brand">开发者文档</Badge>
          <h1 className="mt-4 text-[40px] font-semibold leading-tight">{docs.brand?.name || "TokenHub"} 接入文档</h1>
          <p className="mt-3 text-base text-ink-secondary">
            正文栏约 720px。代码落在碳面上。示例带当前品牌 Base URL；Key 用占位，不写平台成本。
          </p>
          <p className="mt-4 font-mono text-[13px] text-ink-mute">Base URL · {docs.brand?.api_domain || "api.tokenhub.local"}</p>
          <p className="mt-2 text-sm text-ink">可用模型：{(docs.models || []).join("、") || "接通后显示白名单"}</p>
        </div>
        <section id="curl" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="text-lg font-semibold">curl</h2>
          <CodeBlock>{docs.examples?.curl || "curl 示例接通后出现。Key 用 sk-...xxxx。"}</CodeBlock>
        </section>
        <section id="python" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="text-lg font-semibold">Python</h2>
          <CodeBlock>{docs.examples?.python || "Python 示例接通后出现。"}</CodeBlock>
        </section>
        <section id="node" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="text-lg font-semibold">Node.js</h2>
          <CodeBlock>{docs.examples?.node || "Node 示例接通后出现。"}</CodeBlock>
        </section>
        <section id="messages" className="scroll-mt-24 flex flex-col gap-3">
          <h2 className="text-lg font-semibold">Anthropic Messages</h2>
          <CodeBlock>{docs.examples?.messages || "Messages 示例接通后出现。"}</CodeBlock>
        </section>
        {docs.examples?.video ? (
          <section className="flex flex-col gap-3">
            <h2 className="text-lg font-semibold">视频任务</h2>
            <CodeBlock>{docs.examples.video}</CodeBlock>
          </section>
        ) : null}
        <div className="space-y-2 text-sm text-ink-secondary">
          <p>{docs.notes?.errors}</p>
          <p>{docs.notes?.rate_limit}</p>
          <p>{docs.notes?.webhook}</p>
        </div>
      </article>
    </main>
  );
}
