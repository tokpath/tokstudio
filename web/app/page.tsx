import { fetchAPI } from "@/lib/api";
import type { Brand } from "@/lib/brand";
import { summarizeReady, type Readyz } from "@/lib/status";
import { headers } from "next/headers";
import Link from "next/link";
import PublicStorefront from "./storefront";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

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
  const vendors = Array.from(new Set(models.map((item) => item.vendor).filter(Boolean))) as string[];

  return (
    <main>
      <section className="relative overflow-hidden">
        <div className="th-grid pointer-events-none absolute inset-0" />
        <div className="th-noise pointer-events-none absolute inset-0 opacity-60" />
        <div className="relative mx-auto flex max-w-6xl flex-col gap-8 px-6 pb-16 pt-14 md:pt-20">
          <p className="text-sm uppercase tracking-[0.22em]" style={{ color: "var(--brand-primary)" }}>
            公共站点 · {host}
          </p>
          <div className="max-w-3xl">
            <Badge tone="brand">一个 Key · 多模型 · 可审计账单</Badge>
            <h1 className="mt-4 text-5xl font-semibold tracking-tight md:text-6xl">
              {brandName}
              <span className="mt-3 block text-3xl font-medium text-slate-300 md:text-4xl">
                3 分钟，接入 <span style={{ color: "var(--brand-primary)" }}>多模型</span>
              </span>
            </h1>
            <p className="mt-5 max-w-2xl text-lg leading-8 text-slate-300">
              面向开发者的多模型 API 中转。用一个 Base URL 和一把 Key 接入模型。注册时绑定的渠道不能自己改，这是为了保证分销归因可审计。
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <Button asChild>
                <Link href="/login">获取 API Key</Link>
              </Button>
              <Button asChild variant="outline">
                <Link href="#models">探索模型</Link>
              </Button>
              <Button asChild variant="ghost">
                <Link href="/docs">看接入文档</Link>
              </Button>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-3">
            <Stat label="可用模型" value={String(models.length || "—")} hint="按当前域名白名单展示" />
            <Stat label="可用性" value={readyOk ? "就绪" : "检查中"} hint={summary} />
            <Stat label="延迟目标" value="~300ms" hint="控制面健康检查通过后再跑流量" />
          </div>
          <div className="grid gap-3 md:grid-cols-3">
            <CategoryCard title="文本模型" body="Chat、Responses、Anthropic Messages，一套账单。" href="#models" />
            <CategoryCard title="图像与视频" body="媒体任务走异步网关，结果只给签名 URL。" href="#plans" />
            <CategoryCard title="渠道与分销" body="A/B/C 归因写死，佣金按实际消耗冻结结算。" href="/partner" />
          </div>
        </div>
      </section>

      {vendors.length > 0 ? (
        <section className="border-y border-white/5 bg-black/20 py-6">
          <p className="mb-3 text-center text-xs uppercase tracking-[0.2em] text-slate-500">当前目录里的供应商</p>
          <div className="overflow-hidden">
            <div className="flex min-w-max animate-th-marquee gap-10 px-6 text-sm text-slate-400">
              {[...vendors, ...vendors].map((vendor, index) => (
                <span key={`${vendor}-${index}`} className="whitespace-nowrap">
                  {vendor}
                </span>
              ))}
            </div>
          </div>
        </section>
      ) : null}

      <section className="mx-auto max-w-6xl px-6 py-16">
        <PublicStorefront models={models} plans={plans} />
      </section>

      <section className="mx-auto grid max-w-6xl gap-6 px-6 pb-20 md:grid-cols-3">
        <WhyCard title="只路由，不混账" body="每次请求都能追到用户、Key、渠道、价格版本和 usage。重复 webhook 不会双扣。" />
        <WhyCard title="渠道归属不可自改" body="注册时写死归因。前端没有切换渠道入口，后端再拦一层 403。" />
        <WhyCard title="OEM 换皮不换账" body="独立域名替换 Logo、主色和文档 Base URL，账务事实源仍是 TokenHub。" />
      </section>
    </main>
  );
}

function Stat({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-5 py-4">
      <p className="text-xs uppercase tracking-[0.16em] text-slate-500">{label}</p>
      <p className="mt-2 text-2xl font-semibold">{value}</p>
      <p className="mt-1 text-xs text-slate-500">{hint}</p>
    </div>
  );
}

function CategoryCard({ title, body, href }: { title: string; body: string; href: string }) {
  return (
    <Link href={href} className="group rounded-2xl border border-white/10 bg-white/[0.03] p-5 transition hover:border-white/20 hover:bg-white/[0.05]">
      <p className="text-lg font-medium">{title}</p>
      <p className="mt-2 text-sm leading-6 text-slate-400">{body}</p>
      <p className="mt-4 text-sm" style={{ color: "var(--brand-primary)" }}>
        查看 →
      </p>
    </Link>
  );
}

function WhyCard({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-6">
      <h2 className="text-lg font-medium">{title}</h2>
      <p className="mt-2 text-sm leading-6 text-slate-400">{body}</p>
    </div>
  );
}
