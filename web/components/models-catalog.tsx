"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/empty-state";
import {
  capabilityLabels,
  formatContext,
  formatMoney,
  inferKind,
  priceForModel,
  type CatalogModel,
} from "@/lib/catalog";

const FILTERS = [
  { id: "all", label: "全部" },
  { id: "text", label: "文本" },
  { id: "image", label: "图像" },
  { id: "video", label: "视频" },
  { id: "embedding", label: "向量" },
  { id: "audio", label: "转写" },
] as const;

function CopyId({ id }: { id: string }) {
  return (
    <button
      type="button"
      className="rounded-control border border-hairline px-1.5 py-0.5 font-mono text-[11px] text-ink-mute hover:text-ink"
      title="复制模型 ID"
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        void navigator.clipboard.writeText(id);
      }}
    >
      复制
    </button>
  );
}

export function ModelsCatalog({ models }: { models: CatalogModel[] }) {
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<(typeof FILTERS)[number]["id"]>("all");
  const [view, setView] = useState<"list" | "table">("list");

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return models.filter((m) => {
      const kind = inferKind(m);
      if (filter !== "all" && kind !== filter) return false;
      if (!needle) return true;
      const hay = `${m.id} ${m.display_name} ${m.vendor} ${m.description || ""}`.toLowerCase();
      return hay.includes(needle);
    });
  }, [models, q, filter]);

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="搜索模型…"
          className="lg:max-w-md"
          aria-label="搜索模型"
        />
        <div className="flex flex-wrap items-center gap-2">
          {FILTERS.map((f) => {
            const count = f.id === "all" ? models.length : models.filter((m) => inferKind(m) === f.id).length;
            if (f.id !== "all" && count === 0) return null;
            const active = filter === f.id;
            return (
              <button
                key={f.id}
                type="button"
                onClick={() => setFilter(f.id)}
                className={`rounded-control px-3 py-1.5 text-sm ${
                  active ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-secondary"
                }`}
              >
                {f.label} {count}
              </button>
            );
          })}
          <Badge>{filtered.length} 个模型</Badge>
          <div className="ml-auto flex gap-1">
            <button
              type="button"
              onClick={() => setView("list")}
              className={`rounded-control px-2 py-1 text-[12px] ${view === "list" ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute"}`}
            >
              列表
            </button>
            <button
              type="button"
              onClick={() => setView("table")}
              className={`rounded-control px-2 py-1 text-[12px] ${view === "table" ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute"}`}
            >
              表格
            </button>
          </div>
        </div>
      </div>

      {filtered.length === 0 ? (
        <div className="rounded-stamp border border-hairline bg-canvas-raised">
          <EmptyState title="没有匹配的模型" detail="换个关键词，或清掉筛选再试。" />
        </div>
      ) : view === "table" ? (
        <div className="overflow-x-auto rounded-stamp border border-hairline bg-canvas-raised">
          <table className="min-w-[960px] w-full text-left text-sm">
            <thead className="border-b border-hairline">
              <tr>
                {["模型", "厂商", "上下文", "输入", "输出", "能力"].map((h) => (
                  <th key={h} className="th-eyebrow px-4 py-3 text-ink-mute">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-hairline">
              {filtered.map((m) => {
                const price = priceForModel(m);
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
                    <td className="px-4 py-3 font-mono tabular-nums">{price.secondary}</td>
                    <td className="px-4 py-3 text-[12px] text-ink-mute">{capabilityLabels(m.capabilities).join(" · ") || "—"}</td>
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
            const caps = capabilityLabels(m.capabilities);
            const price = priceForModel(m);
            return (
              <li key={m.id}>
                <Link
                  href={`/models/${m.id}`}
                  className="block rounded-stamp border border-hairline bg-canvas-raised p-4 no-underline transition-colors hover:bg-brand-soft/30"
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="th-eyebrow text-ink-mute">{kind}</span>
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
                        {kind === "text" ? `出 ${price.secondary}` : price.secondary}
                      </p>
                    </div>
                  </div>

                  <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[13px] text-ink-secondary">
                    <span>
                      上下文 <span className="font-mono tabular-nums text-ink">{formatContext(m.context_length)}</span>
                    </span>
                    {m.max_completion_tokens ? (
                      <span>
                        最大输出 <span className="font-mono tabular-nums text-ink">{formatContext(m.max_completion_tokens)}</span>
                      </span>
                    ) : null}
                    {kind === "text" ? (
                      <>
                        <span>
                          输入 <span className="font-mono tabular-nums text-ink">{formatMoney(m.sell_price?.input)}</span>
                        </span>
                        <span>
                          输出 <span className="font-mono tabular-nums text-ink">{formatMoney(m.sell_price?.output)}</span>
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
                          {c}
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
