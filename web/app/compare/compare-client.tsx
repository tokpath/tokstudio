"use client";

import { useMemo, useState } from "react";
import {
  capabilityLabels,
  formatContext,
  inferKind,
  priceForModel,
  type CatalogModel,
} from "@/lib/catalog";

function pickDefault(models: CatalogModel[], index: number) {
  const text = models.filter((m) => inferKind(m) === "text");
  const pool = text.length >= 2 ? text : models;
  return pool[Math.min(index, Math.max(pool.length - 1, 0))]?.id || "";
}

export function ModelCompare({ models }: { models: CatalogModel[] }) {
  const [leftId, setLeftId] = useState(() => pickDefault(models, 0));
  const [rightId, setRightId] = useState(() => pickDefault(models, 1));
  const left = useMemo(() => models.find((m) => m.id === leftId), [models, leftId]);
  const right = useMemo(() => models.find((m) => m.id === rightId), [models, rightId]);

  const rows: { label: string; take: (m?: CatalogModel) => string }[] = [
    { label: "模型 ID", take: (m) => m?.id || "—" },
    { label: "厂商", take: (m) => m?.vendor || "—" },
    { label: "模态", take: (m) => (m ? inferKind(m) : "—") },
    { label: "上下文", take: (m) => formatContext(m?.context_length) },
    { label: "输入 / 主价", take: (m) => (m ? priceForModel(m).primary : "—") },
    { label: "输出 / 次价", take: (m) => (m ? priceForModel(m).secondary : "—") },
    { label: "能力", take: (m) => capabilityLabels(m?.capabilities).join(" · ") || "—" },
    { label: "状态", take: (m) => (m?.status || "available").toUpperCase() },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 md:grid-cols-2">
        <label className="flex flex-col gap-1 text-xs text-ink-mute">
          左侧
          <select
            aria-label="对比左侧模型"
            className="h-10 rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink"
            value={leftId}
            onChange={(e) => setLeftId(e.target.value)}
          >
            {models.map((m) => (
              <option key={`l-${m.id}`} value={m.id}>
                {m.display_name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-ink-mute">
          右侧
          <select
            aria-label="对比右侧模型"
            className="h-10 rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink"
            value={rightId}
            onChange={(e) => setRightId(e.target.value)}
          >
            {models.map((m) => (
              <option key={`r-${m.id}`} value={m.id}>
                {m.display_name}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised">
        <table className="min-w-full text-left text-sm">
          <thead className="border-b border-hairline">
            <tr>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">维度</th>
              <th className="px-4 py-3 font-semibold">{left?.display_name || "—"}</th>
              <th className="px-4 py-3 font-semibold">{right?.display_name || "—"}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-hairline">
            {rows.map((row) => (
              <tr key={row.label} className="hover:bg-brand-soft/40">
                <td className="px-4 py-3 text-ink-secondary">{row.label}</td>
                <td className="px-4 py-3 font-mono text-[13px] tabular-nums">{row.take(left)}</td>
                <td className="px-4 py-3 font-mono text-[13px] tabular-nums">{row.take(right)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
