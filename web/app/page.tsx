import { CodeBlock } from "@/components/code-block";
import { Eyebrow } from "@/components/eyebrow";
import { LedgerSection } from "@/components/ledger-section";
import { SiteHeader } from "@/components/site-header";
import { StatusBadge } from "@/components/status-badge";
import { formatHealthCurl, listHealthFields } from "@/lib/health";
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
  const healthRows = listHealthFields(health);
  const requestId = "error" in ready ? undefined : ready.request_id;
  const curl = formatHealthCurl(apiBase);

  return (
    <div className="flex min-h-screen flex-col">
      <SiteHeader />
      <main className="mx-auto flex w-full max-w-[1120px] flex-1 flex-col gap-8 px-6 py-16">
        <section className="flex max-w-3xl flex-col gap-4">
          <Eyebrow className="text-brand-emphasis">Status</Eyebrow>
          <h1 className="text-[40px] font-semibold leading-tight text-ink">
            一个 Key，可解释路由，账能复算。
          </h1>
          <p className="text-base text-ink-secondary">
            这是 M0 公共状态页。探活写成回单，状态带字；示例 curl 只用品牌 Base URL，不内嵌 Key。
          </p>
        </section>

        <section id="receipt" className="scroll-mt-20 rounded-stamp border border-hairline bg-canvas-raised p-6">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
            <Eyebrow>Control receipt</Eyebrow>
            <StatusBadge badge={readyBadge} />
          </div>
          <p className="mb-3 text-base text-ink">{readySummary}</p>
          <p className="font-mono text-[13px] leading-relaxed text-ink-secondary">{receipt}</p>
          {requestId ? (
            <p className="mt-3 font-mono text-[13px] tabular-nums text-ink-mute">request {requestId}</p>
          ) : null}
        </section>

        <LedgerSection
          title="就绪检查"
          eyebrow="Ledger"
          rows={checks}
          emptyTitle="还没有就绪检查"
          emptyDetail="控制面回单会在 /readyz 接通后出现。"
        />

        <LedgerSection
          title="健康字段"
          eyebrow="Health"
          rows={healthRows}
          emptyTitle="还没有健康字段"
          emptyDetail="控制面回单会在 /healthz 接通后出现。"
        />

        <section className="flex max-w-3xl flex-col gap-3">
          <h2 className="text-lg font-semibold">接入示例</h2>
          <p className="text-sm text-ink-secondary">当前品牌 Base URL 上的探活。完整 Key 不会写进示例。</p>
          <CodeBlock>{curl}</CodeBlock>
        </section>

        <section className="grid gap-6 md:grid-cols-2">
          <div className="flex flex-col gap-3">
            <h2 className="text-lg font-semibold">健康原文</h2>
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
