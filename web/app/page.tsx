import Link from "next/link";
import { headers } from "next/headers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { PublicSection, StatStrip } from "@/components/public-section";
import PublicStorefront from "./storefront";
import { fetchAPI } from "@/lib/api";
import {
  MEDIA_WALL,
  VENDOR_MARQUEE,
  formatMoney,
  inferKind,
  loadCatalog,
  priceForModel,
  type CatalogModel,
} from "@/lib/catalog";
import { loadSite } from "@/lib/site-content";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default async function PublicHome() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const site = await loadSite(host);
  let plans: { id: string; name: string; price_minor: number }[] = [];
  try {
    const data = await fetchAPI<{ items: { id: string; name: string; price_minor: number }[] }>("/v1/plans");
    plans = data.items || [];
  } catch {
    plans = [];
  }
  const text = models.filter((m) => inferKind(m) === "text");
  const image = models.filter((m) => inferKind(m) === "image");
  const video = models.filter((m) => inferKind(m) === "video");
  const featuredText = pickFeatured(text, [
    "openai/gpt-5.6-sol",
    "anthropic/claude-fable-5",
    "google/gemini-3.7-flash",
    "x-ai/grok-4.6",
    "deepseek/deepseek-v4-flash-vision-exp",
    "bailian/qwen3.8-max",
    "z-ai/glm-5.3",
    "moonshotai/kimi-k3",
    "bytedance/seedance-2.5",
    "openai/gpt-image-2",
  ]);
  const featuredVideo = video.slice(0, 5);
  const cheapest = [...text]
    .filter((m) => Number(m.sell_price?.input) > 0)
    .sort((a, b) => Number(a.sell_price?.input) - Number(b.sell_price?.input))[0];

  return (
    <main className="flex w-full flex-col">
      {/* 促销条：细线纸面，不用渐变胶囊 */}
      <div className="border-b border-hairline bg-canvas-raised">
        <div className="mx-auto flex max-w-[1120px] items-center justify-between gap-3 px-6 py-2 text-[13px]">
          <p className="text-ink-secondary">
            <span className="th-eyebrow text-hold">PROMO</span>
            <span className="ml-3">Seedance 2.5 最高立减 20% · 720p $0.24/秒起</span>
          </p>
          <Link href="/models/bytedance/seedance-2.5" className="shrink-0 text-brand-emphasis no-underline">
            查看详情 →
          </Link>
        </div>
      </div>

      <div className="mx-auto flex w-full max-w-[1120px] flex-col gap-16 px-6 py-14">
        {/* Hero：价目 + curl 气质，区块对齐 ofox */}
        <section className="grid gap-10 lg:grid-cols-[1.2fr_0.8fr] lg:items-start">
          <div className="flex flex-col gap-5">
            <Badge tone="brand">100+ 模型，一个 API Key</Badge>
            <h1 className="text-[40px] font-semibold leading-[1.15] tracking-tight">
              3 分钟，接入<span className="text-brand-emphasis"> 世界模型</span>
            </h1>
            <p className="text-base text-brand-emphasis">官方渠道 · 稳定高速 · 账能复算</p>
            <StatStrip
              items={[
                { label: "顶级模型", value: `${models.length}+` },
                { label: "可用性", value: "99.9%" },
                { label: "延迟", value: "~300ms", hint: "边缘加速" },
              ]}
            />
            <div className="flex flex-wrap gap-3">
              <Button asChild>
                <Link href="/login">获取 API Key</Link>
              </Button>
              <Button asChild variant="outline">
                <Link href="/models">探索模型</Link>
              </Button>
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              <CategoryLink href="/models?kind=text" title="编程模型" detail="推理、重构与 Agent。" count={text.length} />
              <CategoryLink href="/image" title="图像模型" detail="生成、编辑与视觉创作。" count={image.length} />
              <CategoryLink href="/video" title="视频模型" detail="文生视频与音视频。" count={video.length} />
            </div>
          </div>

          <aside className="rounded-stamp border border-hairline bg-canvas-raised p-5">
            <p className="th-eyebrow text-hold">FEATURED</p>
            <h2 className="mt-2 text-lg font-semibold">Seedance 2.5</h2>
            <p className="mt-2 text-sm text-ink-secondary">
              单段最长 30 秒，自带同步音频。720p $0.24/秒起 · 1080p $0.48/秒。
            </p>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src="https://ofox.ai/landing-assets/gc-wall/seedance-2-5-c-v1.webp"
              alt=""
              className="mt-4 aspect-[4/3] w-full rounded-control border border-hairline object-cover"
            />
            <Button asChild className="mt-4 w-full">
              <Link href="/models/bytedance/seedance-2.5">试试 Seedance 2.5</Link>
            </Button>
          </aside>
        </section>

        {/* 厂商跑马灯 */}
        <section className="overflow-hidden border-y border-hairline py-6">
          <p className="mb-4 text-center text-[13px] text-ink-mute">
            全球主流云厂商 · {models.length}+ 大模型一站接入
          </p>
          <div className="flex flex-wrap items-center justify-center gap-x-8 gap-y-3 text-sm font-medium text-ink-secondary">
            {VENDOR_MARQUEE.map((v) => (
              <span key={v}>{v}</span>
            ))}
          </div>
        </section>

        {/* 图像与视频墙 */}
        <PublicSection
          eyebrow="IMAGE & VIDEO"
          title="不止于文本模型，视频图像同样出色"
          description="Seedance 系列、GPT Image、Seedream、Wan。结构对齐 ofox 媒体墙；皮肤是纸面细线。"
          action={
            <div className="flex gap-2">
              <Button asChild variant="outline" size="sm">
                <Link href="/image">浏览图像</Link>
              </Button>
              <Button asChild variant="outline" size="sm">
                <Link href="/video">浏览视频</Link>
              </Button>
            </div>
          }
        >
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            {MEDIA_WALL.map((item) => (
              <Link
                key={item.id + item.src}
                href={`/models/${item.id}`}
                className="group overflow-hidden rounded-stamp border border-hairline bg-canvas-raised no-underline"
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={item.src} alt="" className="aspect-[4/3] w-full object-cover" />
                <div className="px-3 py-2">
                  <p className="th-eyebrow text-ink-mute">{item.kind}</p>
                  <p className="mt-1 text-sm font-medium text-ink group-hover:text-brand-emphasis">{item.name}</p>
                </div>
              </Link>
            ))}
          </div>
        </PublicSection>

        {/* 多模态演示面板 */}
        <PublicSection
          eyebrow="MULTIMODAL API"
          title="文本、图像、视频，一个 Key 搞定"
          description="只需换端点和模型 ID。图像与视频有公开价目可查。"
        >
          <div className="rounded-stamp border border-hairline bg-canvas-raised p-4">
            <div className="mb-3 flex gap-2">
              {["视频", "图像", "文本"].map((tab, i) => (
                <span
                  key={tab}
                  className={`rounded-control px-3 py-1.5 text-sm ${i === 0 ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute"}`}
                >
                  {tab}
                </span>
              ))}
            </div>
            <CodeBlock>{`curl ${apiBase}/v1/videos \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"bytedance/seedance-2.5","prompt":"A quiet street at dusk"}'`}</CodeBlock>
            <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {featuredVideo.map((m) => (
                <Link
                  key={m.id}
                  href={`/models/${m.id}`}
                  className="rounded-control border border-hairline px-3 py-2 text-sm no-underline hover:bg-brand-soft/40"
                >
                  <p className="font-medium text-ink">{m.display_name}</p>
                  <p className="font-mono text-[12px] text-brand-emphasis">{priceForModel(m).primary} 起</p>
                </Link>
              ))}
              <Link href="/video" className="rounded-control border border-dashed border-hairline px-3 py-2 text-sm text-ink-secondary no-underline">
                全部视频模型 →
              </Link>
            </div>
          </div>
        </PublicSection>

        {/* 价目精选 */}
        <PublicSection
          eyebrow="PRICE BOOK"
          title={cheapest ? `前沿大模型，低至 ${formatMoney(cheapest.sell_price?.input)}` : "已发布价目"}
          description="实时价格，所见即所付。金额等宽，不含上游成本。"
          action={
            <Button asChild variant="outline" size="sm">
              <Link href="/models">查看全部模型</Link>
            </Button>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            {featuredText.slice(0, 10).map((m) => (
              <Link
                key={m.id}
                href={`/models/${m.id}`}
                className="flex items-start justify-between gap-3 rounded-stamp border border-hairline bg-canvas-raised p-4 no-underline hover:bg-brand-soft/30"
              >
                <div className="min-w-0">
                  <p className="font-medium text-ink">{m.display_name}</p>
                  <p className="mt-1 font-mono text-[12px] text-ink-mute">{m.id}</p>
                </div>
                <div className="shrink-0 text-right">
                  <button
                    type="button"
                    className="pointer-events-none rounded-control border border-hairline px-2 py-0.5 font-mono text-[11px] text-ink-mute"
                  >
                    复制 ID
                  </button>
                  <p className="mt-2 font-mono text-[13px] tabular-nums text-brand-emphasis">
                    入 {formatMoney(m.sell_price?.input)}
                  </p>
                  <p className="font-mono text-[12px] tabular-nums text-ink-mute">出 {formatMoney(m.sell_price?.output)}</p>
                </div>
              </Link>
            ))}
          </div>
          <div className="flex flex-wrap gap-3">
            <Button asChild variant="outline" size="sm">
              <Link href="/best-value">性价比模型</Link>
            </Button>
            <Button asChild variant="outline" size="sm">
              <Link href="/models">查看全部模型</Link>
            </Button>
          </div>
        </PublicSection>

        {/* 排行榜 */}
        <PublicSection
          eyebrow="LEADERBOARD"
          title="大家都在用什么？"
          description={site.leaderboards?.note || "结构对齐 ofox 用量榜。份额来自公开站快照，接入真实汇总后替换。"}
          action={
            <Button asChild variant="outline" size="sm">
              <Link href="/leaderboards/models">完整榜单</Link>
            </Button>
          }
        >
          <ol className="divide-y divide-hairline rounded-stamp border border-hairline bg-canvas-raised">
            {(site.leaderboards?.models || []).map((row) => (
              <li key={row.rank}>
                <Link href={row.id ? `/models/${row.id}` : "/models"} className="flex items-center gap-4 px-4 py-3 no-underline hover:bg-brand-soft/30">
                  <span className="font-mono text-sm text-ink-mute">{row.rank}</span>
                  <div className="min-w-0 flex-1">
                    <p className="text-[12px] text-ink-mute">{row.vendor}</p>
                    <p className="font-medium text-ink">{row.name}</p>
                  </div>
                  <span className="font-mono tabular-nums text-ink">{row.share}</span>
                  <span className="w-20 text-right font-mono text-[12px] tabular-nums text-ink-mute">{row.delta}</span>
                </Link>
              </li>
            ))}
          </ol>
        </PublicSection>

        {/* 工具接入 */}
        <PublicSection
          eyebrow="TOOLS"
          title="常用工具，只需换一个 URL"
          description="不用迁移 SDK。在现有配置里改 base URL。"
        >
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {[
              { href: "/quickstart", title: "3 分钟快速上手", meta: "curl / Python / Node" },
              { href: "/vibe-coding", title: "Claude Code", meta: "ANTHROPIC_BASE_URL" },
              { href: "/vibe-coding", title: "Codex", meta: "~/.codex/config.toml" },
              { href: "/docs", title: "OpenCode", meta: "内置服务商" },
              { href: "/docs", title: "Cline", meta: "自定义 OpenAI 端点" },
              { href: "/docs", title: "Python · Node · cURL", meta: "base_url" },
            ].map((item) => (
              <Link
                key={item.title}
                href={item.href}
                className="rounded-stamp border border-hairline bg-canvas-raised p-4 no-underline hover:bg-brand-soft/30"
              >
                <p className="font-medium text-ink">{item.title}</p>
                <p className="mt-1 font-mono text-[12px] text-ink-mute">{item.meta}</p>
              </Link>
            ))}
          </div>
        </PublicSection>

        {/* 为什么选 */}
        <PublicSection eyebrow="WHY" title="为什么选 TokenHub" description="只说做得到的——每一条都能在产品里核对。">
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            {[
              { t: "平台费 0%", d: "标价即实付，公开价目可查。", href: "/models" },
              { t: "可用性 99.9%", d: "多上游自动容灾，attempt 可解释。", href: "/verify" },
              { t: "延迟 ~300ms", d: "边缘加速，全球接入。", href: "/enterprise" },
              { t: "透明公开", d: "价格与用量结构公开可见。", href: "/leaderboards/models" },
              { t: "只路由，不存储", d: "同步 API 不落业务正文。", href: "/trust" },
              { t: "企业", d: "团队管控、渠道额度、OEM 换章。", href: "/enterprise" },
            ].map((card) => (
              <Link
                key={card.t}
                href={card.href}
                className="rounded-stamp border border-hairline bg-canvas-raised p-5 no-underline hover:bg-brand-soft/30"
              >
                <p className="text-lg font-semibold text-ink">{card.t}</p>
                <p className="mt-2 text-sm text-ink-secondary">{card.d}</p>
              </Link>
            ))}
          </div>
        </PublicSection>

        {/* CTA */}
        <section className="rounded-stamp border border-hairline bg-canvas-raised px-6 py-12 text-center">
          <p className="th-eyebrow text-brand-emphasis">GET STARTED</p>
          <h2 className="mt-3 text-2xl font-semibold">3 分钟，接入世界模型</h2>
          <p className="mx-auto mt-2 max-w-lg text-sm text-ink-secondary">改一个 base URL 就能跑。按量付费。</p>
          <div className="mt-6 flex flex-wrap justify-center gap-3">
            <Button asChild>
              <Link href="/login">获取 API Key</Link>
            </Button>
            <Button asChild variant="outline">
              <Link href="/models">探索模型</Link>
            </Button>
          </div>
        </section>

        {/* D33 公共站账本入口：套餐 / 充值，结构保留给未登录购买 */}
        <PublicStorefront models={models.slice(0, 6)} plans={plans} />
      </div>
    </main>
  );
}

function CategoryLink({
  href,
  title,
  detail,
  count,
}: {
  href: string;
  title: string;
  detail: string;
  count: number;
}) {
  return (
    <Link
      href={href}
      className="rounded-stamp border border-hairline bg-canvas-raised p-4 no-underline hover:bg-brand-soft/40"
    >
      <p className="th-eyebrow text-ink-mute">{count} 个</p>
      <p className="mt-1 font-semibold text-ink">{title}</p>
      <p className="mt-1 text-[13px] text-ink-secondary">{detail}</p>
    </Link>
  );
}

function pickFeatured(models: CatalogModel[], preferred: string[]) {
  const byId = new Map(models.map((m) => [m.id, m]));
  const picked: CatalogModel[] = [];
  for (const id of preferred) {
    const m = byId.get(id);
    if (m) picked.push(m);
  }
  for (const m of models) {
    if (picked.length >= 10) break;
    if (!picked.find((p) => p.id === m.id)) picked.push(m);
  }
  return picked;
}
