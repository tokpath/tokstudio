import Link from "next/link";
import { headers } from "next/headers";
import { ArrowRight, Image, Sparkles, Video } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { RoutingReceipt } from "@/components/routing-receipt";
import { PublicSection, StatStrip } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { IconStamp } from "@/components/icon-stamp";
import PublicStorefront from "./storefront";
import { fetchAPI } from "@/lib/api";
import { getTranslations } from "next-intl/server";
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
import { iconForHref, iconForTool, WHY_ICONS } from "@/lib/page-icons";
import type { LucideIcon } from "lucide-react";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default async function PublicHome() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const t = await getTranslations("home");
  const tc = await getTranslations("common");
  const tCat = await getTranslations("catalog");
  const priceUnits = { perSec: tCat("perSec"), perImage: tCat("perImage") };
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
      <div className="border-b border-hairline bg-canvas-raised">
        <div className="mx-auto flex max-w-[1200px] items-center justify-between gap-4 px-6 py-2.5 text-[13px]">
          <p className="text-ink-secondary">
            <span className="th-eyebrow text-hold">HOLD</span>
            <span className="ml-3 leading-relaxed">{t("holdBanner")}</span>
          </p>
          <Link href="/models/bytedance/seedance-2.5" className="shrink-0 text-brand-emphasis no-underline transition-colors duration-150 hover:underline">
            {t("viewPrice")}
          </Link>
        </div>
      </div>

      <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-24 px-6 py-20">
        <section className="grid gap-12 lg:grid-cols-[1.15fr_0.85fr] lg:items-start">
          <div>
            <Badge tone="brand">Status</Badge>
            <h1 className="th-display mt-5">
              {t("h1a")}
              <span className="text-brand-emphasis">{t("h1b")}</span>
              {t("h1c")}
            </h1>
            <p className="mt-6 max-w-xl text-base leading-relaxed text-ink-secondary">{t("lead")}</p>
            <div className="mt-10">
              <StatStrip
                items={[
                  { label: t("statModels"), value: String(models.length || "—"), hint: t("statModelsHint") },
                  { label: t("statPrice"), value: cheapest ? formatMoney(cheapest.sell_price?.input) : "—", hint: t("statPriceHint") },
                  { label: t("statSite"), value: host, hint: t("statSiteHint") },
                ]}
              />
            </div>
            <div className="mt-10 flex flex-wrap items-center gap-3">
              <Button asChild>
                <Link href="/login">{t("ctaStart")}</Link>
              </Button>
              <Button asChild variant="outline">
                <Link href="/models">{t("ctaPrice")}</Link>
              </Button>
              <Link href="/docs" className="px-2 text-sm text-ink-secondary no-underline transition-colors duration-150 hover:text-ink">
                {t("ctaDocs")}
              </Link>
            </div>
            <div className="mt-12 grid gap-4 sm:grid-cols-3">
              <CategoryLink href="/models?kind=text" title={t("catCode")} detail={t("catCodeDetail")} countLabel={tc("countItems", { count: text.length })} icon={iconForHref("/models?kind=text")} />
              <CategoryLink href="/image" title={t("catImage")} detail={t("catImageDetail")} countLabel={tc("countItems", { count: image.length })} icon={Image} />
              <CategoryLink href="/video" title={t("catVideo")} detail={t("catVideoDetail")} countLabel={tc("countItems", { count: video.length })} icon={Video} />
            </div>
          </div>

          <aside className="rounded-card border border-hairline bg-canvas-raised p-6">
            <p className="th-eyebrow text-hold">FEATURED</p>
            <h2 className="mt-3 text-lg font-semibold leading-snug">Seedance 2.5</h2>
            <p className="mt-3 text-sm leading-relaxed text-ink-secondary">{t("featuredBody")}</p>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src="https://ofox.ai/landing-assets/gc-wall/seedance-2-5-c-v1.webp"
              alt=""
              className="mt-5 aspect-[4/3] w-full rounded-control border border-hairline object-cover"
            />
            <Button asChild variant="outline" className="mt-5 w-full">
              <Link href="/models/bytedance/seedance-2.5">{t("featuredCta")}</Link>
            </Button>
          </aside>
        </section>

        <section className="overflow-hidden border-y border-hairline py-8" aria-label={t("vendorsAria")}>
          <p className="th-eyebrow text-ink-mute">Published vendors</p>
          <div className="mt-5 flex flex-wrap items-center gap-x-10 gap-y-3 text-sm text-ink-secondary">
            {VENDOR_MARQUEE.map((v) => (
              <span key={v}>{v}</span>
            ))}
          </div>
        </section>

        <section className="grid gap-6 lg:grid-cols-2 lg:gap-10">
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold">{t("sampleTitle")}</h2>
            <p className="text-sm leading-relaxed text-ink-secondary">{t("sampleLead")}</p>
            <CodeBlock>{`curl ${apiBase}/v1/models \\\n  -H "Authorization: Bearer sk-...xxxx"`}</CodeBlock>
          </div>
          <RoutingReceipt
            eyebrow="EXAMPLE · ATTEMPT"
            lines={[
              "public_model tokenhub/echo-1",
              "attempt 1 → echo-primary → 429 rate_limited",
              "attempt 2 → echo-backup → 200",
            ]}
            footnote={t("sampleFoot")}
          />
        </section>

        <PublicSection
          eyebrow="IMAGE & VIDEO"
          title={t("mediaTitle")}
          description={t("mediaLead")}
          action={
            <div className="flex gap-3">
              <Button asChild variant="outline" size="sm">
                <Link href="/image">{t("browseImage")}</Link>
              </Button>
              <Button asChild variant="outline" size="sm">
                <Link href="/video">{t("browseVideo")}</Link>
              </Button>
            </div>
          }
        >
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            {MEDIA_WALL.map((item) => (
              <Link
                key={item.id + item.src}
                href={`/models/${item.id}`}
                className="group overflow-hidden rounded-card border border-hairline bg-canvas-raised no-underline"
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={item.src} alt="" className="aspect-[4/3] w-full object-cover" />
                <div className="px-4 py-3">
                  <p className="th-eyebrow text-ink-mute">{item.kind}</p>
                  <p className="mt-1.5 text-sm font-medium text-ink group-hover:text-brand-emphasis">{item.name}</p>
                </div>
              </Link>
            ))}
          </div>
        </PublicSection>

        <PublicSection
          eyebrow="MULTIMODAL API"
          title={t("multiTitle")}
          description={t("multiLead")}
        >
          <div className="rounded-card border border-hairline bg-canvas-raised p-4">
            <div className="mb-4 flex gap-2">
              {[t("tabVideo"), t("tabImage"), t("tabText")].map((tab, i) => (
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
                  <p className="font-mono text-[12px] text-brand-emphasis">{tc("fromPrice", { price: priceForModel(m, priceUnits).primary })}</p>
                </Link>
              ))}
              <Link href="/video" className="rounded-control border border-dashed border-hairline px-3 py-2 text-sm text-ink-secondary no-underline">
                {t("allVideo")}
              </Link>
            </div>
          </div>
        </PublicSection>

        <PublicSection
          eyebrow="PRICE BOOK"
          title={cheapest ? t("priceTitleCheap", { price: formatMoney(cheapest.sell_price?.input) }) : t("priceTitle")}
          description={t("priceLead")}
          action={
            <Button asChild variant="outline" size="sm">
              <Link href="/models">{t("allModels")}</Link>
            </Button>
          }
        >
          <div className="grid gap-4 sm:grid-cols-2">
            {featuredText.slice(0, 10).map((m) => (
              <Link
                key={m.id}
                href={`/models/${m.id}`}
                className="flex items-start justify-between gap-4 rounded-card border border-hairline bg-canvas-raised px-5 py-4 no-underline transition-colors duration-150 hover:bg-brand-soft/30"
              >
                <div className="min-w-0">
                  <p className="font-medium text-ink">{m.display_name}</p>
                  <p className="mt-1 font-mono text-[12px] text-ink-mute">{m.id}</p>
                </div>
                <div className="shrink-0 text-right">
                  <p className="font-mono text-[13px] tabular-nums text-brand-emphasis">{tc("inPrice", { price: formatMoney(m.sell_price?.input) })}</p>
                  <p className="font-mono text-[12px] tabular-nums text-ink-mute">{tc("outPrice", { price: formatMoney(m.sell_price?.output) })}</p>
                </div>
              </Link>
            ))}
          </div>
          <div className="flex flex-wrap gap-3">
            <Button asChild variant="outline" size="sm">
              <Link href="/best-value">{t("bestValue")}</Link>
            </Button>
            <Button asChild variant="outline" size="sm">
              <Link href="/models">{t("allModels")}</Link>
            </Button>
          </div>
        </PublicSection>

        <PublicSection
          eyebrow="LEADERBOARD"
          title={t("boardTitle")}
          description={site.leaderboards?.note || t("boardLead")}
          action={
            <Button asChild variant="outline" size="sm">
              <Link href="/leaderboards/models">{t("boardCta")}</Link>
            </Button>
          }
        >
          <ol className="divide-y divide-hairline rounded-card border border-hairline bg-canvas-raised">
            {(site.leaderboards?.models || []).map((row) => (
              <li key={row.rank}>
                <Link href={row.id ? `/models/${row.id}` : "/models"} className="flex items-center gap-4 px-5 py-3.5 no-underline transition-colors duration-150 hover:bg-brand-soft/30">
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

        <PublicSection eyebrow="TOOLS" title={t("toolsTitle")} description={t("toolsLead")}>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[
              { href: "/quickstart", title: t("toolQs"), meta: "curl / Python / Node", icon: Sparkles },
              { href: "/vibe-coding", title: "Claude Code", meta: "ANTHROPIC_BASE_URL", icon: iconForTool("Claude Code") },
              { href: "/vibe-coding", title: "Codex", meta: "~/.codex/config.toml", icon: iconForTool("Codex") },
              { href: "/docs", title: t("toolOpencode"), meta: t("toolOpencodeMeta"), icon: iconForTool("OpenCode") },
              { href: "/docs", title: t("toolCline"), meta: t("toolClineMeta"), icon: iconForTool("Cline") },
              { href: "/docs", title: "Python · Node · cURL", meta: "base_url", icon: iconForTool("Python · Node · cURL") },
            ].map((item) => (
              <FeatureCard
                key={item.title}
                href={item.href}
                icon={item.icon}
                title={item.title}
                meta={item.meta}
              />
            ))}
          </div>
        </PublicSection>

        <PublicSection eyebrow="WHY" title={t("whyTitle")} description={t("whyLead")}>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {[
              { k: "why0t", d: "why0d", href: "/models" },
              { k: "why1t", d: "why1d", href: "/verify" },
              { k: "why2t", d: "why2d", href: "/enterprise" },
              { k: "why3t", d: "why3d", href: "/leaderboards/models" },
              { k: "why4t", d: "why4d", href: "/trust" },
              { k: "why5t", d: "why5d", href: "/enterprise" },
            ].map((card, i) => (
              <FeatureCard
                key={card.k}
                href={card.href}
                icon={WHY_ICONS[i]}
                title={t(card.k)}
                description={t(card.d)}
              />
            ))}
          </div>
        </PublicSection>

        {/* D33 公共站账本入口：套餐 / 充值，结构保留给未登录购买 */}
        <PublicStorefront models={models.slice(0, 12)} plans={plans} />

        <section className="flex max-w-2xl flex-col py-4">
          <h2 className="th-display-sm">{t("ctaTitle")}</h2>
          <p className="mt-5 text-base leading-relaxed text-ink-secondary">{t("ctaLead")}</p>
          <div className="mt-8 flex flex-wrap gap-3">
            <Button asChild>
              <Link href="/login">{t("ctaStart")}</Link>
            </Button>
            <Button asChild variant="outline">
              <Link href="/models">
                {t("ctaPrice")}
                <ArrowRight />
              </Link>
            </Button>
          </div>
        </section>
      </div>
    </main>
  );
}

function CategoryLink({
  href,
  title,
  detail,
  countLabel,
  icon: Icon,
}: {
  href: string;
  title: string;
  detail: string;
  countLabel: string;
  icon: LucideIcon;
}) {
  return (
    <Link href={href} className="rounded-card border border-hairline bg-canvas-raised p-5 no-underline transition-colors duration-150 hover:bg-brand-soft/40">
      <div className="flex items-start justify-between gap-3">
        <p className="th-eyebrow text-ink-mute">{countLabel}</p>
        <IconStamp icon={Icon} size="sm" />
      </div>
      <p className="mt-4 font-semibold text-ink">{title}</p>
      <p className="mt-1.5 text-[13px] leading-relaxed text-ink-secondary">{detail}</p>
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
