"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Copy, LayoutList, Search, Table2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/empty-state";
import { iconForKind } from "@/lib/page-icons";
import {
  capabilityLabels,
  catalogHref,
  formatContext,
  formatMoney,
  inferKind,
  priceForModel,
  type CatalogFacets,
  type CatalogModel,
  type CatalogQuery,
} from "@/lib/catalog";

const FILTER_IDS = ["all", "text", "image", "video", "embedding", "audio"] as const;

function CopyId({ id }: { id: string }) {
  const t = useTranslations("catalog");
  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 rounded-control border border-hairline px-1.5 py-0.5 font-mono text-[11px] text-ink-mute hover:text-ink"
      title={t("copyId")}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        void navigator.clipboard.writeText(id);
      }}
    >
      {t("copy")}
      <Copy className="size-3" strokeWidth={1.75} aria-hidden />
    </button>
  );
}

export function ModelsCatalog({
  models,
  facets,
  query,
  basePath = "/models",
}: {
  models: CatalogModel[];
  facets: CatalogFacets;
  query: CatalogQuery;
  basePath?: string;
}) {
  const t = useTranslations("catalog");
  const tCaps = useTranslations("caps");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const units = { perSec: t("perSec"), perImage: t("perImage") };
  const activeKind = query.kind || "all";
  const [q, setQ] = useState(query.q || "");
  const [view, setView] = useState<"list" | "table">("list");

  useEffect(() => {
    setQ(query.q || "");
  }, [query.q]);

  useEffect(() => {
    const handle = window.setTimeout(() => {
      const next = q.trim() || undefined;
      const current = query.q || undefined;
      if (next === current) {
        return;
      }
      router.replace(catalogHref(basePath, { vendor: query.vendor, kind: query.kind, q: next }), { scroll: false });
    }, 400);
    return () => window.clearTimeout(handle);
  }, [q, basePath, query.kind, query.vendor, query.q, router]);

  const hrefOf = (patch: CatalogQuery) =>
    catalogHref(basePath, { vendor: query.vendor, kind: query.kind, q: q.trim() || undefined, ...patch });

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) {
      return models;
    }
    return models.filter((m) => {
      const hay = `${m.id} ${m.display_name} ${m.vendor} ${m.description || ""}`.toLowerCase();
      return hay.includes(needle);
    });
  }, [models, q]);

  function kindCount(id: string) {
    if (id === "all") {
      return facets.kinds.reduce((sum, item) => sum + item.count, 0);
    }
    return facets.kinds.find((item) => item.id === id)?.count ?? 0;
  }

  function capText(caps?: Record<string, unknown>) {
    return capabilityLabels(caps)
      .map((key) => tCaps(key as "vision"))
      .join(" · ");
  }

  const vendors = facets.vendors;
  const vendorMissing =
    query.vendor && !vendors.some((item) => item.id === query.vendor)
      ? [{ id: query.vendor, count: filtered.length }]
      : [];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <div className="relative max-w-md">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-mute" strokeWidth={1.75} aria-hidden />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t("search")}
            className="pl-9"
            aria-label={t("searchAria")}
          />
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {FILTER_IDS.map((id) => {
            const count = kindCount(id);
            if (id !== "all" && count === 0 && activeKind !== id) return null;
            const active = activeKind === id;
            const KindIcon = iconForKind(id);
            return (
              <Link
                key={id}
                href={hrefOf({ kind: id === "all" ? undefined : id })}
                className={`inline-flex items-center gap-1.5 rounded-control px-3 py-1.5 text-sm no-underline transition-colors duration-150 ${
                  active ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-secondary hover:bg-canvas-raised hover:text-ink"
                }`}
              >
                <KindIcon className="size-3.5" strokeWidth={1.75} aria-hidden />
                {t(id)} {count}
              </Link>
            );
          })}
          <Badge>{t("count", { count: filtered.length })}</Badge>
          <div className="ml-auto flex gap-1">
            <button
              type="button"
              onClick={() => setView("list")}
              className={`inline-flex items-center gap-1.5 rounded-control px-2.5 py-1.5 text-[12px] ${view === "list" ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute hover:text-ink"}`}
            >
              <LayoutList className="size-3.5" strokeWidth={1.75} aria-hidden />
              {t("list")}
            </button>
            <button
              type="button"
              onClick={() => setView("table")}
              className={`inline-flex items-center gap-1.5 rounded-control px-2.5 py-1.5 text-[12px] ${view === "table" ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute hover:text-ink"}`}
            >
              <Table2 className="size-3.5" strokeWidth={1.75} aria-hidden />
              {t("table")}
            </button>
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2" aria-label={t("vendorsAria")}>
        <Link
          href={hrefOf({ vendor: undefined })}
          className={`rounded-control px-3 py-1.5 text-sm no-underline transition-colors duration-150 ${
            !query.vendor ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-secondary hover:bg-canvas-raised hover:text-ink"
          }`}
        >
          {t("allVendors")}
        </Link>
        {[...vendorMissing, ...vendors].map((item) => {
          const active = query.vendor === item.id;
          return (
            <Link
              key={item.id}
              href={hrefOf({ vendor: active ? undefined : item.id })}
              className={`rounded-control px-3 py-1.5 text-sm no-underline transition-colors duration-150 ${
                active ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-secondary hover:bg-canvas-raised hover:text-ink"
              }`}
            >
              {item.id} {item.count}
            </Link>
          );
        })}
      </div>

      {filtered.length === 0 ? (
        <div className="rounded-card border border-hairline bg-canvas-raised">
          <EmptyState title={t("emptyTitle")} detail={t("emptyDetail")} />
        </div>
      ) : view === "table" ? (
        <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
          <table className="min-w-[960px] w-full text-left text-sm">
            <thead className="border-b border-hairline">
              <tr>
                {(["colModel", "colVendor", "colContext", "colInput", "colOutput", "colCaps"] as const).map((h) => (
                  <th key={h} className="th-eyebrow px-4 py-3 text-ink-mute">
                    {t(h)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-hairline">
              {filtered.map((m) => {
                const kind = inferKind(m);
                const price = priceForModel(m, units);
                return (
                  <tr key={m.id} className="hover:bg-brand-soft/40">
                    <td className="px-4 py-3">
                      <Link href={`/models/${m.id}`} className="font-medium no-underline hover:text-brand-emphasis">
                        {m.display_name}
                      </Link>
                      <p className="font-mono text-[11px] text-ink-mute">{m.id}</p>
                    </td>
                    <td className="px-4 py-3 text-ink-secondary">{m.vendor}</td>
                    <td className="px-4 py-3 font-mono tabular-nums">{formatContext(m.context_length)}</td>
                    <td className="px-4 py-3 font-mono tabular-nums text-brand-emphasis">{price.primary}</td>
                    <td className="px-4 py-3 font-mono tabular-nums">
                      {kind === "video" ? t("kindVideo") : kind === "image" ? t("kindImage") : price.secondary}
                    </td>
                    <td className="px-4 py-3 text-[12px] text-ink-mute">{capText(m.capabilities) || "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <ul className="flex flex-col gap-3">
          {filtered.map((m) => {
            const kind = inferKind(m);
            const KindIcon = iconForKind(kind);
            const caps = capabilityLabels(m.capabilities);
            const price = priceForModel(m, units);
            return (
              <li key={m.id}>
                <Link
                  href={`/models/${m.id}`}
                  className="block rounded-card border border-hairline bg-canvas-raised px-5 py-4 no-underline transition-colors duration-150 hover:bg-brand-soft/30"
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="inline-flex items-center gap-1 th-eyebrow text-ink-mute">
                          <KindIcon className="size-3" strokeWidth={1.75} aria-hidden />
                          {kind}
                        </span>
                        <h3 className="text-base font-semibold text-ink">{m.display_name}</h3>
                        <span className="th-eyebrow text-success">{(m.status || "available").toUpperCase()}</span>
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        <span className="font-mono text-[12px] text-ink-mute">{m.id}</span>
                        <CopyId id={m.id} />
                        <span className="text-[12px] text-ink-mute">{m.vendor}</span>
                      </div>
                    </div>
                    <div className="text-right">
                      <p className="font-mono text-sm font-medium tabular-nums text-brand-emphasis">{price.primary}</p>
                      <p className="font-mono text-[12px] tabular-nums text-ink-mute">
                        {kind === "text"
                          ? tCommon("outPrice", { price: price.secondary })
                          : kind === "video"
                            ? t("kindVideo")
                            : kind === "image"
                              ? t("kindImage")
                              : price.secondary}
                      </p>
                    </div>
                  </div>

                  <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[13px] text-ink-secondary">
                    <span>
                      {t("context")} <span className="font-mono tabular-nums text-ink">{formatContext(m.context_length)}</span>
                    </span>
                    {m.max_completion_tokens ? (
                      <span>
                        {t("maxOut")} <span className="font-mono tabular-nums text-ink">{formatContext(m.max_completion_tokens)}</span>
                      </span>
                    ) : null}
                    {kind === "text" ? (
                      <>
                        <span>
                          {t("input")} <span className="font-mono tabular-nums text-ink">{formatMoney(m.sell_price?.input)}</span>
                        </span>
                        <span>
                          {t("output")} <span className="font-mono tabular-nums text-ink">{formatMoney(m.sell_price?.output)}</span>
                        </span>
                      </>
                    ) : null}
                  </div>

                  {m.description ? (
                    <p className="mt-2 line-clamp-2 text-[13px] text-ink-mute">{m.description}</p>
                  ) : null}

                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {caps.length ? (
                      caps.map((c) => (
                        <span key={c} className="rounded-control border border-hairline px-2 py-0.5 text-[11px] text-ink-secondary">
                          {tCaps(c as "vision")}
                        </span>
                      ))
                    ) : (
                      <span className="text-[11px] text-ink-mute">—</span>
                    )}
                  </div>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
