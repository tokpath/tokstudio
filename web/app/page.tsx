import { summarizeReady, type Readyz } from "@/lib/status";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

async function fetchJSON<T>(path: string): Promise<T | { error: string }> {
  try {
    const response = await fetch(`${apiBase}${path}`, { cache: "no-store" });
    return (await response.json()) as T;
  } catch (error) {
    return { error: error instanceof Error ? error.message : "无法连接 API" };
  }
}

export default async function HomePage() {
  const health = await fetchJSON<Record<string, string>>("/healthz");
  const ready = await fetchJSON<Readyz>("/readyz");
  const readySummary = "error" in ready ? `API 不可达：${ready.error}` : summarizeReady(ready);

  return (
    <main className="mx-auto flex max-w-3xl flex-col gap-6 px-6 py-16">
      <p className="text-sm uppercase tracking-[0.2em] text-cyan-300">TokenHub M0</p>
      <h1 className="text-4xl font-semibold">控制面健康状态</h1>
      <p className="text-slate-300">
        这是第一版公共状态页。M1 会按域名拆成公共站、用户控制台、渠道控制台和管理控制台。
      </p>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-2 text-lg font-medium">健康检查</h2>
        <pre className="overflow-x-auto text-sm text-cyan-100">{JSON.stringify(health, null, 2)}</pre>
      </section>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-2 text-lg font-medium">就绪检查</h2>
        <p className="mb-3 text-slate-200">{readySummary}</p>
        <pre className="overflow-x-auto text-sm text-cyan-100">{JSON.stringify(ready, null, 2)}</pre>
      </section>
    </main>
  );
}
