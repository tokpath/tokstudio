import { fetchAPI } from "@/lib/api";
import type { Brand } from "@/lib/brand";
import { summarizeReady, type Readyz } from "@/lib/status";
import { headers } from "next/headers";
import PublicStorefront from "./storefront";

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
    <main className="mx-auto flex max-w-5xl flex-col gap-6 px-6 py-12">
      <p className="text-sm uppercase tracking-[0.2em]" style={{ color: "var(--brand-primary)" }}>
        公共站点 · {host}
      </p>
      <h1 className="text-4xl font-semibold">{brandName}</h1>
      <p className="max-w-2xl text-slate-300">
        面向开发者的多模型 API 中转。用一个 Base URL 和一把 Key 接入模型。注册时绑定的渠道不能自己改，这是为了保证分销归因可审计。
      </p>
      <p className="text-slate-200">{summary}</p>
      <PublicStorefront models={models} plans={plans} />
    </main>
  );
}
