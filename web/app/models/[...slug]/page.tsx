import Link from "next/link";
import { headers } from "next/headers";
import { notFound } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { ModelPlayground } from "@/components/model-playground";
import {
  capabilityLabels,
  formatContext,
  formatMoney,
  inferKind,
  loadCatalog,
  priceForModel,
} from "@/lib/catalog";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default async function ModelDetailPage({
  params,
}: {
  params: Promise<{ slug: string[] }>;
}) {
  const { slug } = await params;
  const id = decodeURIComponent(slug.join("/"));
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const model = models.find((m) => m.id === id);
  if (!model) notFound();

  const kind = inferKind(model);
  const caps = capabilityLabels(model.capabilities);
  const price = priceForModel(model);
  const relatedVendor = models.filter((m) => m.id !== model.id && m.vendor === model.vendor).slice(0, 4);
  const relatedKind = models.filter((m) => m.id !== model.id && inferKind(m) === kind).slice(0, 4);

  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-10 px-6 py-12">
      <nav className="text-[13px] text-ink-mute" aria-label="Breadcrumb">
        <Link href="/" className="no-underline hover:text-ink">
          首页
        </Link>
        {" / "}
        <Link href="/models" className="no-underline hover:text-ink">
          模型
        </Link>
        {" / "}
        <Link href={`/models?vendor=${model.vendor}`} className="no-underline hover:text-ink">
          {model.vendor}
        </Link>
        {" / "}
        <span className="text-ink">{model.display_name}</span>
      </nav>

      <header className="flex flex-col gap-5 border-b border-hairline pb-8 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="brand">{kind}</Badge>
            <span className="th-eyebrow text-success">{(model.status || "available").toUpperCase()}</span>
          </div>
          <h1 className="mt-3 text-[40px] font-semibold leading-tight">{model.display_name}</h1>
          <p className="mt-2 font-mono text-[13px] text-ink-mute">{model.id}</p>
          {model.description ? (
            <p className="mt-4 max-w-3xl text-sm leading-relaxed text-ink-secondary">{model.description}</p>
          ) : null}
          <div className="mt-4 flex flex-wrap gap-2">
            {caps.map((c) => (
              <span key={c} className="rounded-control border border-hairline px-2 py-1 text-[12px] text-ink-secondary">
                {c}
              </span>
            ))}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <Button asChild variant="outline">
            <Link href="/models">返回目录</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href={`/models?kind=${kind}`}>同类型</Link>
          </Button>
          <Button asChild>
            <Link href="/login">开始使用</Link>
          </Button>
        </div>
      </header>

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="上下文" value={formatContext(model.context_length)} />
        <Stat label="最大输出" value={formatContext(model.max_completion_tokens)} />
        <Stat label={kind === "video" ? "视频单价" : kind === "image" ? "图像单价" : "输入"} value={price.primary} />
        <Stat label={kind === "text" ? "输出" : "厂商"} value={kind === "text" ? formatMoney(model.sell_price?.output) : model.vendor} />
      </section>

      <section className="grid gap-4 lg:grid-cols-2">
        <div className="rounded-stamp border border-hairline bg-canvas-raised p-5">
          <p className="th-eyebrow text-ink-mute">PROVIDER · SELL PRICE</p>
          <h2 className="mt-2 text-lg font-semibold">公开卖价</h2>
          <div className="mt-4 grid grid-cols-2 gap-3 text-sm">
            <PriceCell label="输入 /M" value={formatMoney(model.sell_price?.input)} />
            <PriceCell label="输出 /M" value={formatMoney(model.sell_price?.output)} />
            <PriceCell label="媒体 /秒" value={formatMoney(model.sell_price?.media, "/秒")} />
            <PriceCell label="图像" value={formatMoney(model.sell_price?.image, "/张")} />
          </div>
          <p className="mt-4 text-[13px] text-ink-mute">不展示上游成本。客户账只看卖价。</p>
        </div>
        <div className="rounded-stamp border border-hairline bg-canvas-raised p-5">
          <p className="th-eyebrow text-ink-mute">ROUTING RECEIPT</p>
          <h2 className="mt-2 text-lg font-semibold">可解释路径（示意）</h2>
          <p className="mt-4 font-mono text-[13px] leading-relaxed text-ink">
            {model.id}
            <br />→ published sell_price
            <br />→ attempt 1 provider whitelist
            <br />→ customer ledger one charge
          </p>
          <p className="mt-3 text-[13px] text-ink-secondary">真实 attempt 在登录后的用户台查看；此处不伪造 Playground 对话。</p>
        </div>
      </section>

      <ModelPlayground
        modelId={model.id}
        displayName={model.display_name}
        kind={kind}
        sampleCurl={
          kind === "video"
            ? `curl ${apiBase}/v1/videos \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"A quiet street at dusk"}'`
            : kind === "image"
              ? `curl ${apiBase}/v1/images/generations \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"a brass diving helmet on wet dock planks"}'`
              : `curl ${apiBase}/v1/chat/completions \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","messages":[{"role":"user","content":"hello"}]}'`
        }
      />

      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">代码示例（固定）</h2>
        <CodeBlock>
          {kind === "video"
            ? `curl ${apiBase}/v1/videos \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"A quiet street at dusk"}'`
            : kind === "image"
              ? `curl ${apiBase}/v1/images/generations \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"a brass diving helmet on wet dock planks"}'`
              : `curl ${apiBase}/v1/chat/completions \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","messages":[{"role":"user","content":"hello"}]}'`}
        </CodeBlock>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">常见问题</h2>
        <div className="divide-y divide-hairline rounded-stamp border border-hairline bg-canvas-raised">
          {[
            {
              q: `在 TokenHub 上使用 ${model.display_name} 需要多少钱？`,
              a: `公开卖价：输入 ${formatMoney(model.sell_price?.input)}，输出 ${formatMoney(model.sell_price?.output)}。以价目页实时数字为准。`,
            },
            {
              q: `${model.display_name} 的上下文窗口是多少？`,
              a: `上下文 ${formatContext(model.context_length)}，最大输出 ${formatContext(model.max_completion_tokens)}。`,
            },
            {
              q: `如何调用 ${model.display_name}？`,
              a: `注册拿 Key，把 base URL 换成品牌 API 域名，model 字段填 ${model.id}。`,
            },
            {
              q: `支持哪些能力？`,
              a: caps.length ? caps.join("、") : "以目录能力标签为准。",
            },
          ].map((item) => (
            <details key={item.q} className="group px-4 py-3">
              <summary className="cursor-pointer list-none font-medium text-ink">{item.q}</summary>
              <p className="mt-2 text-sm text-ink-secondary">{item.a}</p>
            </details>
          ))}
        </div>
      </section>

      {relatedVendor.length ? (
        <Related title={`更多 ${model.vendor} 模型`} items={relatedVendor} />
      ) : null}
      {relatedKind.length ? <Related title="类似模型" items={relatedKind} /> : null}
    </main>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-stamp border border-hairline bg-canvas-raised px-4 py-4">
      <p className="th-eyebrow text-ink-mute">{label}</p>
      <p className="mt-2 font-mono text-lg font-medium tabular-nums">{value}</p>
    </div>
  );
}

function PriceCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-control border border-hairline px-3 py-3">
      <p className="th-eyebrow text-ink-mute">{label}</p>
      <p className="mt-1 font-mono text-base tabular-nums text-brand-emphasis">{value}</p>
    </div>
  );
}

function Related({
  title,
  items,
}: {
  title: string;
  items: { id: string; display_name: string; sell_price?: Record<string, unknown> }[];
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      <ul className="divide-y divide-hairline rounded-stamp border border-hairline bg-canvas-raised">
        {items.map((item) => (
          <li key={item.id}>
            <Link href={`/models/${item.id}`} className="flex justify-between gap-3 px-4 py-3 text-sm no-underline hover:bg-brand-soft/40">
              <span className="font-medium">{item.display_name}</span>
              <span className="font-mono text-ink-mute">
                {formatMoney(item.sell_price?.input)} / {formatMoney(item.sell_price?.output)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
