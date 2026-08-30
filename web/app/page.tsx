import { ThemeToggle } from "@/components/theme-toggle";
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
  const readyIsOk = !("error" in ready) && ready.status === "ready";

  return (
    <main className="mx-auto flex max-w-3xl flex-col gap-6 px-6 py-16">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex flex-col gap-3">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-brand-emphasis">
            Status
          </p>
          <h1 className="text-4xl font-semibold tracking-tight">控制面健康状态</h1>
        </div>
        <ThemeToggle />
      </header>
      <p className="text-ink-secondary">
        这是第一版公共状态页。M1 会按域名拆成公共站、用户控制台、渠道控制台和管理控制台；浅色、深色和跟随系统在四个入口共用。
      </p>
      <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
        <h2 className="mb-2 text-lg font-medium">健康检查</h2>
        <pre className="overflow-x-auto font-mono text-sm text-ink-secondary">
          {JSON.stringify(health, null, 2)}
        </pre>
      </section>
      <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <h2 className="text-lg font-medium">就绪检查</h2>
          <span
            className={`rounded-control px-2 py-0.5 font-mono text-[11px] font-medium uppercase tracking-[0.08em] ${
              readyIsOk ? "text-success" : "text-hold"
            }`}
          >
            {readyIsOk ? "Ready" : "Hold"}
          </span>
        </div>
        <p className="mb-3 text-ink">{readySummary}</p>
        <pre className="overflow-x-auto font-mono text-sm text-ink-secondary">
          {JSON.stringify(ready, null, 2)}
        </pre>
      </section>
    </main>
  );
}
