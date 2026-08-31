import Link from "next/link";
import { headers } from "next/headers";
import { fetchAPI } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicPageHero, PublicSection, StatStrip } from "@/components/public-section";

export default async function QuickstartPage() {
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
      <PublicPageHero
        eyebrow="QUICKSTART"
        title="3 步接入你的 Agent"
        description="从 0 到生产环境，3 分钟搞定。结构对齐 ofox 快速开始：注册 → 复制示例 → 开始构建。"
        primaryHref="/login"
        primaryLabel="获取 API Key"
        secondaryHref="/docs"
        secondaryLabel="完整文档"
      />

      <StatStrip
        items={[
          { label: "步骤", value: "3", hint: "注册 → 复制 → 调用" },
          { label: "协议", value: "OpenAI 兼容", hint: "另有 Messages" },
          { label: "Base URL", value: base, hint: "当前品牌域名" },
        ]}
      />

      <div className="grid gap-4 lg:grid-cols-3">
        {[
          { n: "1", t: "获取 API Key", d: "注册并一键生成 Key。完整密钥默认掩码。", href: "/login", cta: "去注册 / 登录" },
          { n: "2", t: "接入代码", d: "复制示例，3 分钟完成接入。Key 用占位。", href: "#code", cta: "看示例" },
          { n: "3", t: "开始构建", d: "看价目、看用量、看路由回单。", href: "/app", cta: "打开用户台" },
        ].map((s) => (
          <div key={s.n} className="rounded-card border border-hairline bg-canvas-raised p-5">
            <p className="th-eyebrow text-brand-emphasis">STEP {s.n}</p>
            <h3 className="mt-2 text-lg font-semibold">{s.t}</h3>
            <p className="mt-2 text-sm text-ink-secondary">{s.d}</p>
            <Button asChild variant="outline" size="sm" className="mt-4">
              <Link href={s.href}>{s.cta}</Link>
            </Button>
          </div>
        ))}
      </div>

      <PublicSection id="code" eyebrow="STEP 2" title="接入代码" description="OpenAI 兼容。换 base URL 与 model id 即可。">
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

      <PublicSection eyebrow="NEXT" title="接下来">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {[
            { href: "/models", label: "模型目录" },
            { href: "/vibe-coding", label: "编程工具接入" },
            { href: "/docs", label: "完整文档" },
            { href: "/enterprise", label: "企业能力" },
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
        <h2 className="text-2xl font-semibold">准备好构建你的下一个 AI Agent 了吗？</h2>
        <p className="mt-2 text-sm text-ink-secondary">3 分钟接入，立即开始</p>
        <div className="mt-6 flex flex-wrap justify-center gap-3">
          <Button asChild>
            <Link href="/login">获取 API Key</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/models">探索模型</Link>
          </Button>
        </div>
      </section>
    </main>
  );
}
