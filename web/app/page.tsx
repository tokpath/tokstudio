import { fetchAPI } from "@/lib/api";
import type { Brand } from "@/lib/brand";
import { summarizeReady, type Readyz } from "@/lib/status";
import { headers } from "next/headers";
import Link from "next/link";
import PublicStorefront from "./storefront";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { RoutingReceipt } from "@/components/routing-receipt";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default async function PublicHome() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let brandName = "TokenHub";
  try {
    const data = await fetchAPI<{ brand: Brand }>("/v1/public/brand", { host });
    brandName = data.brand?.name || brandName;
  } catch {
    /* keep default */
  }
  let ready: Readyz | { error: string } = { error: "API 不可达" };
  try {
    ready = await fetchAPI<Readyz>("/readyz");
  } catch (error) {
    ready = { error: error instanceof Error ? error.message : "API 不可达" };
  }
  const summary = "error" in ready ? ready.error : summarizeReady(ready);
  const readyOk = !("error" in ready) && ready.status === "ready";
  let plans: { id: string; name: string; price_minor: number }[] = [];
  try {
    const data = await fetchAPI<{ items: { id: string; name: string; price_minor: number }[] }>("/v1/plans");
    plans = data.items || [];
  } catch {
    plans = [];
  }
  let models: { id?: string; display_name?: string; vendor?: string }[] = [];
  try {
    const data = await fetchAPI<{ items: { id?: string; display_name?: string; vendor?: string }[] }>("/v1/public/models", {
      host,
    });
    models = data.items || [];
  } catch {
    models = [];
  }
  const vendors = [...new Set(models.map((model) => model.vendor).filter((vendor): vendor is string => Boolean(vendor)))];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-24 px-6 py-20">
      <section className="flex max-w-3xl flex-col gap-6">
        <Badge tone="brand">Status</Badge>
        <h1 className="text-[36px] font-semibold leading-[1.08] sm:text-[56px]">
          一个 Key，<span className="text-brand-emphasis">可解释路由</span>，账能复算。
        </h1>
        <p className="max-w-xl text-base leading-relaxed text-ink-secondary">
          {brandName} 公布已发布价目，示例只用品牌 Base URL。注册时绑定的渠道不能自己改。
        </p>
        <div className="flex flex-wrap gap-x-10 gap-y-6 pt-2">
          <Stat label="可用模型" value={String(models.length || "—")} hint="按当前域名白名单" />
          <Stat label="控制面" value={readyOk ? "READY" : "HOLD"} hint={summary} />
          <Stat label="站点" value={host} hint="OEM 只换章和 Logo" />
        </div>
        <div className="flex flex-wrap gap-3 pt-2">
          <Button asChild>
            <Link href="/login">开始使用</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="#models">看价目</Link>
          </Button>
          <Button asChild variant="ghost">
            <Link href="/docs">文档</Link>
          </Button>
        </div>
      </section>

      {vendors.length > 0 ? (
        <section className="border-y border-hairline py-6" aria-label="已发布厂商">
          <p className="th-eyebrow text-ink-mute">Published vendors</p>
          <div className="mt-4 flex flex-wrap gap-x-8 gap-y-2 text-sm text-ink-secondary">
            {vendors.map((vendor) => (
              <span key={vendor}>{vendor}</span>
            ))}
          </div>
        </section>
      ) : null}

      <section className="grid gap-4 lg:grid-cols-2">
        <div className="flex flex-col gap-3">
          <h2 className="text-lg font-semibold">接入示例</h2>
          <p className="text-sm text-ink-secondary">完整 Key 不会写进示例。右侧是一张 EXAMPLE 回单，不是实时账。</p>
          <CodeBlock>{`curl ${apiBase}/v1/models \\\n  -H "Authorization: Bearer sk-...xxxx"`}</CodeBlock>
        </div>
        <RoutingReceipt
          eyebrow="EXAMPLE · ATTEMPT"
          lines={[
            "public_model tokenhub/echo-1",
            "attempt 1 → echo-primary → 429 rate_limited",
            "attempt 2 → echo-backup → 200",
          ]}
          footnote="客户只收一笔"
        />
      </section>

      <PublicStorefront models={models} plans={plans} />

      <section className="flex max-w-2xl flex-col gap-4 py-8">
        <h2 className="text-3xl font-semibold tracking-tight">开始对账</h2>
        <p className="text-base leading-relaxed text-ink-secondary">改一个 Base URL 就能跑。价目已发布，每次 attempt 留得住。</p>
        <div className="flex flex-wrap gap-3">
          <Button asChild>
            <Link href="/login">开始使用</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/docs">看文档</Link>
          </Button>
        </div>
      </section>
    </main>
  );
}

function Stat({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div>
      <p className="th-eyebrow text-ink-mute">{label}</p>
      <p className="mt-2 font-mono text-[32px] font-medium leading-none tabular-nums tracking-tight">{value}</p>
      <p className="mt-2 max-w-[16rem] text-[13px] text-ink-mute">{hint}</p>
    </div>
  );
}
