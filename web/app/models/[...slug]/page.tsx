import Link from "next/link";
import { headers } from "next/headers";
import { notFound } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";
import { ModelPlayground } from "@/components/model-playground";
import {
  capabilityLabels,
  formatContext,
  formatMoney,
  inferKind,
  loadCatalog,
  priceForModel,
} from "@/lib/catalog";
import { getTranslations } from "next-intl/server";
import { PublicMain } from "@/components/public-section";
import { StartUsingLink } from "@/components/start-using-link";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default async function ModelDetailPage({
  params,
}: {
  params: Promise<{ slug: string[] }>;
}) {
  const { slug } = await params;
  const id = decodeURIComponent(slug.join("/"));
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const found = await loadCatalog(host, { id });
  const model = found[0];
  if (!model) notFound();

  const kind = inferKind(model);
  const caps = capabilityLabels(model.capabilities);
  const [relatedVendorRaw, relatedKindRaw] = await Promise.all([
    loadCatalog(host, { vendor: model.vendor, limit: 8 }),
    loadCatalog(host, { kind, limit: 8 }),
  ]);
  const relatedVendor = relatedVendorRaw.filter((m) => m.id !== model.id).slice(0, 4);
  const relatedKind = relatedKindRaw.filter((m) => m.id !== model.id).slice(0, 4);
  const t = await getTranslations("modelDetail");
  const th = await getTranslations("home");
  const tCaps = await getTranslations("caps");
  const tCat = await getTranslations("catalog");
  const priceUnits = { perSec: tCat("perSec"), perImage: tCat("perImage") };
  const price = priceForModel(model, priceUnits);
  const capText = caps.map((c) => tCaps(c as "vision"));

  return (
    <PublicMain>
      <nav className="text-[13px] text-ink-mute" aria-label={t("crumb")}>
        <Link href="/" className="no-underline hover:text-ink">
          {t("home")}
        </Link>
        {" / "}
        <Link href="/models" className="no-underline hover:text-ink">
          {t("models")}
        </Link>
        {" / "}
        <Link href={`/models?vendor=${model.vendor}`} className="no-underline hover:text-ink">
          {model.vendor}
        </Link>
        {" / "}
        <span className="text-ink">{model.display_name}</span>
      </nav>

      <header className="flex flex-col gap-5 border-b border-hairline pb-8 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="brand">{kind}</Badge>
            <span className="th-eyebrow text-success">{(model.status || "available").toUpperCase()}</span>
          </div>
          <h1 className="th-display-sm mt-4">{model.display_name}</h1>
          <p className="mt-3 font-mono text-[13px] text-ink-mute">{model.id}</p>
          {model.description ? (
            <p className="mt-5 max-w-3xl text-base leading-relaxed text-ink-secondary">{model.description}</p>
          ) : null}
          <div className="mt-5 flex flex-wrap gap-2">
            {capText.map((c) => (
              <span key={c} className="rounded-control border border-hairline px-2 py-1 text-[12px] text-ink-secondary">
                {c}
              </span>
            ))}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-3">
          <Button asChild variant="outline">
            <Link href="/models">{t("back")}</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href={`/models?kind=${kind}`}>{t("sameKind")}</Link>
          </Button>
          <Button asChild>
            <StartUsingLink modelId={model.id}>{th("ctaStart")}</StartUsingLink>
          </Button>
        </div>
      </header>

      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label={t("context")} value={formatContext(model.context_length)} />
        <Stat label={t("maxOut")} value={formatContext(model.max_completion_tokens)} />
        <Stat label={kind === "video" ? t("videoPrice") : kind === "image" ? t("imagePrice") : t("input")} value={price.primary} />
        <Stat label={kind === "text" ? t("output") : t("vendor")} value={kind === "text" ? formatMoney(model.sell_price?.output) : model.vendor} />
      </section>

      <section className="grid gap-4 lg:grid-cols-2">
        <div className="rounded-card border border-hairline bg-canvas-raised p-6">
          <p className="th-eyebrow text-ink-mute">PROVIDER · SELL PRICE</p>
          <h2 className="mt-3 text-lg font-semibold">{t("sellTitle")}</h2>
          <div className="mt-4 grid grid-cols-2 gap-3 text-sm">
            <PriceCell label={t("inM")} value={formatMoney(model.sell_price?.input)} />
            <PriceCell label={t("outM")} value={formatMoney(model.sell_price?.output)} />
            <PriceCell label={t("mediaSec")} value={formatMoney(model.sell_price?.media, tCat("perSec"))} />
            <PriceCell label={t("image")} value={formatMoney(model.sell_price?.image, tCat("perImage"))} />
          </div>
          <p className="mt-4 text-[13px] text-ink-mute">{t("sellNote")}</p>
        </div>
        <div className="rounded-card border border-hairline bg-canvas-raised p-6">
          <p className="th-eyebrow text-ink-mute">ROUTING RECEIPT</p>
          <h2 className="mt-3 text-lg font-semibold">{t("routeTitle")}</h2>
          <p className="mt-4 font-mono text-[13px] leading-relaxed text-ink">
            {model.id}
            <br />→ published sell_price
            <br />→ attempt 1 provider whitelist
            <br />→ customer ledger one charge
          </p>
          <p className="mt-3 text-[13px] text-ink-secondary">{t("routeNote")}</p>
        </div>
      </section>

      <ModelPlayground
        modelId={model.id}
        displayName={model.display_name}
        kind={kind}
        sampleCurl={
          kind === "video"
            ? `curl ${apiBase}/v1/videos \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"A quiet street at dusk"}'`
            : kind === "image"
              ? `curl ${apiBase}/v1/images/generations \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"a brass diving helmet on wet dock planks"}'`
              : `curl ${apiBase}/v1/chat/completions \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","messages":[{"role":"user","content":"hello"}]}'`
        }
      />

      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">{t("codeTitle")}</h2>
        <CodeBlock>
          {kind === "video"
            ? `curl ${apiBase}/v1/videos \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"A quiet street at dusk"}'`
            : kind === "image"
              ? `curl ${apiBase}/v1/images/generations \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","prompt":"a brass diving helmet on wet dock planks"}'`
              : `curl ${apiBase}/v1/chat/completions \\\n  -H "Authorization: Bearer sk-...xxxx" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model.id}","messages":[{"role":"user","content":"hello"}]}'`}
        </CodeBlock>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">{t("faqTitle")}</h2>
        <div className="divide-y divide-hairline rounded-card border border-hairline bg-canvas-raised">
          {[
            {
              q: t("faq0q", { name: model.display_name }),
              a: t("faq0a", { input: formatMoney(model.sell_price?.input), output: formatMoney(model.sell_price?.output) }),
            },
            {
              q: t("faq1q", { name: model.display_name }),
              a: t("faq1a", { context: formatContext(model.context_length), maxOut: formatContext(model.max_completion_tokens) }),
            },
            {
              q: t("faq2q", { name: model.display_name }),
              a: t("faq2a", { id: model.id }),
            },
            {
              q: t("faq3q"),
              a: capText.length ? capText.join(" · ") : t("faq3a"),
            },
          ].map((item) => (
            <details key={item.q} className="group px-5 py-4">
              <summary className="cursor-pointer list-none font-medium text-ink">{item.q}</summary>
              <p className="mt-2 text-sm text-ink-secondary">{item.a}</p>
            </details>
          ))}
        </div>
      </section>

      {relatedVendor.length ? (
        <Related title={t("moreVendor", { vendor: model.vendor })} items={relatedVendor} />
      ) : null}
      {relatedKind.length ? <Related title={t("similar")} items={relatedKind} /> : null}
    </PublicMain>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="th-eyebrow text-ink-mute">{label}</p>
      <p className="mt-3 font-mono text-lg font-medium tabular-nums">{value}</p>
    </div>
  );
}

function PriceCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-control border border-hairline px-3 py-3">
      <p className="th-eyebrow text-ink-mute">{label}</p>
      <p className="mt-1 font-mono text-base tabular-nums text-brand-emphasis">{value}</p>
    </div>
  );
}

function Related({
  title,
  items,
}: {
  title: string;
  items: { id: string; display_name: string; sell_price?: Record<string, unknown> }[];
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      <ul className="divide-y divide-hairline rounded-card border border-hairline bg-canvas-raised">
        {items.map((item) => (
          <li key={item.id}>
            <Link href={`/models/${item.id}`} className="flex justify-between gap-4 px-5 py-3.5 text-sm no-underline hover:bg-brand-soft/40">
              <span className="font-medium">{item.display_name}</span>
              <span className="font-mono text-ink-mute">
                {formatMoney(item.sell_price?.input)} / {formatMoney(item.sell_price?.output)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
