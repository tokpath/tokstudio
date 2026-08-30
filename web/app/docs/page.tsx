import type { Metadata } from "next";
import { CodeBlock } from "@/components/code-block";
import { DocsLanguageToggle } from "@/components/docs-language-toggle";
import { Eyebrow } from "@/components/eyebrow";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "文档",
};

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

const sections = [
  { id: "start", label: "快速开始" },
  { id: "auth", label: "认证" },
  { id: "models", label: "模型" },
  { id: "errors", label: "错误码" },
];

export default function DocsPage() {
  return (
    <PublicShell>
      <div className="flex flex-col gap-10 lg:flex-row">
        <nav className="w-full shrink-0 text-sm text-ink-secondary lg:w-48" aria-label="文档目录">
          <div className="mb-4">
            <DocsLanguageToggle />
          </div>
          <Eyebrow className="mb-3 text-ink-mute">Docs</Eyebrow>
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
        <article className="flex max-w-[720px] flex-col gap-12">
          <section className="flex flex-col gap-4">
            <Eyebrow className="text-brand-emphasis">Documentation</Eyebrow>
            <h1 className="text-[40px] font-semibold leading-tight">接入文档</h1>
            <p className="text-base text-ink-secondary">
              正文栏约 720px。代码块落在碳面上。示例只带当前品牌 Base URL 和 Key 占位，不写平台成本。
            </p>
          </section>

          <section id="start" className="scroll-mt-24 flex flex-col gap-3">
            <h2 className="text-lg font-semibold">快速开始</h2>
            <p className="text-base text-ink-secondary">
              用一个 Key 调用已发布模型。路由回单会记下 public_model、provider 和 attempt。客户只收一笔。
            </p>
            <CodeBlock>{`curl ${apiBase}/v1/chat/completions \\
  -H "Authorization: Bearer sk-...xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"public-model","messages":[{"role":"user","content":"ping"}]}'`}</CodeBlock>
          </section>

          <section id="auth" className="scroll-mt-24 flex flex-col gap-3">
            <h2 className="text-lg font-semibold">认证</h2>
            <p className="text-base text-ink-secondary">
              用户台创建的 Key 只显示前缀。复制和轮换会写审计。文档里永远用 <code>sk-...xxxx</code>。
            </p>
          </section>

          <section id="models" className="scroll-mt-24 flex flex-col gap-3">
            <h2 className="text-lg font-semibold">模型</h2>
            <p className="text-base text-ink-secondary">
              只调用已发布的 <span className="font-mono text-[13px]">public_model_id</span>
              。内部上游映射不会出现在这份文档里。
            </p>
          </section>

          <section id="errors" className="scroll-mt-24 flex flex-col gap-3">
            <h2 className="text-lg font-semibold">错误码</h2>
            <p className="text-base text-ink-secondary">
              余额不足写产品文案对应 402，限流对应 429。不要只画红框。身份与账本接通前，控制台会用 HOLD
              字标说明尚未盖章。
            </p>
            <Eyebrow className="text-hold">Deprecated</Eyebrow>
            <p className="text-sm text-ink-mute">废弃接口会留在旧版本文档，并用 HOLD 眉题标成 DEPRECATED。</p>
          </section>
        </article>
      </div>
    </PublicShell>
  );
}
