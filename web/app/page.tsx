import { CheckRow } from "@/components/check-row";
import { CodeBlock } from "@/components/code-block";
import { SiteHeader } from "@/components/site-header";
import { StatusBadge } from "@/components/status-badge";
import {
  badgeForReady,
  formatControlReceipt,
  listChecks,
  summarizeReady,
  type Readyz,
} from "@/lib/status";

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
  const readyBadge = badgeForReady(ready);
  const receipt = formatControlReceipt(ready);
  const checks = listChecks(ready);
  const requestId = "error" in ready ? undefined : ready.request_id;

  return (
    <div className="flex min-h-screen flex-col">
      <SiteHeader />
      <main className="mx-auto flex w-full max-w-[1120px] flex-1 flex-col gap-8 px-6 py-16">
        <section className="flex max-w-3xl flex-col gap-4">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-brand-emphasis">
            Status
          </p>
          <h1 className="text-[40px] font-semibold leading-tight text-ink">
            一个 Key，可解释路由，账能复算。
          </h1>
          <p className="text-base text-ink-secondary">
            这是 M0 公共状态页。控制面探活不是品牌装饰：每次检查都写成回单，状态带字，不靠色点。
          </p>
        </section>

        <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-ink-mute">
              Control receipt
            </p>
            <StatusBadge badge={readyBadge} />
          </div>
          <p className="mb-3 text-base text-ink">{readySummary}</p>
          <p className="font-mono text-[13px] leading-relaxed text-ink-secondary">{receipt}</p>
          {requestId ? (
            <p className="mt-3 font-mono text-[13px] tabular-nums text-ink-mute">request {requestId}</p>
          ) : null}
        </section>

        <section className="overflow-hidden rounded-stamp border border-hairline bg-canvas-raised">
          <div className="flex items-center justify-between border-b border-hairline px-4 py-3">
            <h2 className="text-lg font-semibold">就绪检查</h2>
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-ink-mute">
              Ledger
            </p>
          </div>
          {checks.map((row) => (
            <CheckRow key={row.key} label={row.label} value={row.value} />
          ))}
        </section>

        <section className="grid gap-6 md:grid-cols-2">
          <div className="flex flex-col gap-3">
            <h2 className="text-lg font-semibold">健康检查</h2>
            <CodeBlock>{JSON.stringify(health, null, 2)}</CodeBlock>
          </div>
          <div className="flex flex-col gap-3">
            <h2 className="text-lg font-semibold">就绪原文</h2>
            <CodeBlock>{JSON.stringify(ready, null, 2)}</CodeBlock>
          </div>
        </section>
      </main>
      <footer className="border-t border-hairline">
        <p className="mx-auto max-w-[1120px] px-6 py-8 text-[13px] text-ink-mute">
          TokenHub Clearing · 纸/碳双主题 · M1 将按域名拆成公共站、用户台、渠道台和管理台
        </p>
      </footer>
    </div>
  );
}
