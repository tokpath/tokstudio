import { fetchAPI } from "@/lib/api";
import type { Brand } from "@/lib/brand";
import { summarizeReady, type Readyz } from "@/lib/status";
import { headers } from "next/headers";
import Link from "next/link";
import PublicStorefront from "./storefront";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";

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

  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-16 px-6 py-16">
      <section className="flex max-w-3xl flex-col gap-4">
        <Badge tone="brand">Status</Badge>
        <h1 className="text-[40px] font-semibold leading-tight">一个 Key，可解释路由，账能复算。</h1>
        <p className="text-base text-ink-secondary">
          {brandName} 公布已发布价目，示例只用品牌 Base URL。注册时绑定的渠道不能自己改。
        </p>
        <div className="flex flex-wrap gap-3">
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

      <section className="grid gap-4 md:grid-cols-3">
        <Stat label="可用模型" value={String(models.length || "—")} hint="按当前域名白名单" />
        <Stat label="控制面" value={readyOk ? "READY" : "HOLD"} hint={summary} />
        <Stat label="站点" value={host} hint="OEM 只换章和 Logo" />
      </section>

      <section className="flex max-w-3xl flex-col gap-3">
        <h2 className="text-lg font-semibold">接入示例</h2>
        <p className="text-sm text-ink-secondary">完整 Key 不会写进示例。</p>
        <CodeBlock>{`curl ${apiBase}/v1/models \\\n  -H "Authorization: Bearer sk-...xxxx"`}</CodeBlock>
      </section>

      <PublicStorefront models={models} plans={plans} />
    </main>
  );
}

function Stat({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded-stamp border border-hairline bg-canvas-raised px-4 py-4">
      <p className="th-eyebrow text-ink-mute">{label}</p>
      <p className="mt-2 font-mono text-xl font-medium tabular-nums">{value}</p>
      <p className="mt-1 text-[13px] text-ink-mute">{hint}</p>
    </div>
  );
}
