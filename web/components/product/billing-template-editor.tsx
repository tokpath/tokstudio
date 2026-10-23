"use client";

import { useId, type ReactNode } from "react";
import { useI18n } from "@/components/i18n";
import type { JSONValue } from "@/lib/contracts";
import { FieldHelp } from "./field-help";

type Meter = { modality: string; template: string; variants?: JSONValue[] };
const modalities = [["text", "文本"], ["image", "图片"], ["audio", "音频"], ["video", "视频"]] as const;
const templates: Record<string, [string, string][]> = {
  text: [["tokens", "按 Token"], ["characters", "按字符"]],
  image: [["tokens", "按 Token"], ["images", "按张"], ["megapixels", "按百万像素"]],
  audio: [["tokens", "按 Token"], ["seconds", "按秒"]],
  video: [["tokens", "按 Token"], ["seconds", "按秒"]],
};
const defaultTemplate = (modality: string) => modality === "image" ? "images" : modality === "text" ? "tokens" : "seconds";

export function BillingTemplateEditor({ value, operation, disabled, change, variants }: {
  value: Record<string, JSONValue>; operation?: string; disabled: boolean;
  change: (value: JSONValue) => void;
  variants: (direction: "input" | "output", index: number, value: JSONValue[], change: (value: JSONValue) => void) => ReactNode;
}) {
  const { t } = useI18n();
  const id = useId();
  return <section className="space-y-5">
    <p className="text-sm leading-7 text-ink-secondary">{t("分别设置输入和输出。每个方向使用统一 Token，或按模态组合；同一份内容只计费一次。单价在模型价格中填写。")}</p>
    {(["input", "output"] as const).filter(direction => direction !== "output" || operation !== "embedding").map(direction => {
      const items = (Array.isArray(value[direction]) ? value[direction] : []) as Meter[];
      const mode = !items.length ? "none" : items.some(item => item.modality === "all") ? "all" : "modalities";
      const update = (next: Meter[]) => change({ ...value, [direction]: next as JSONValue[], ...(operation === "embedding" ? { output: [] } : {}) });
      const label = direction === "input" ? "输入计量" : "输出计量";
      return <fieldset key={direction} className="rounded-control border border-hairline p-4" data-config-path={`billing.${direction}`}>
        <legend className="px-2 font-medium">{t(label)}<FieldHelp title={t(label)}>{t("统一 Token 适合上游只提供汇总 Token 的情况。按模态组合时，应确认系统能可靠取得各模态用量；文本、图片、音频、视频各选一种计量方式。")}</FieldHelp></legend>
        <div className="mb-4 flex flex-wrap gap-3">{[["none", "不计费"], ["all", "统一 Token"], ["modalities", "按模态组合"]].map(([key, text]) => <label key={key} className="flex min-h-11 cursor-pointer items-center gap-2 rounded-control border border-hairline px-3 text-sm"><input type="radio" name={`${id}-${direction}`} checked={mode === key} disabled={disabled} onChange={() => update(key === "none" ? [] : key === "all" ? [{ modality: "all", template: "tokens" }] : [{ modality: "text", template: "tokens" }])} />{t(text)}</label>)}</div>
        {mode === "all" && <p className="text-xs leading-6 text-ink-mute">{t("本方向的所有模态合并按 Token 计费，不再叠加图片张数或媒体时长。")}</p>}
        {mode === "modalities" && <div className="space-y-3">{modalities.map(([modality, text]) => {
          const index = items.findIndex(item => item.modality === modality), item = items[index];
          return <div key={modality} className="rounded-control bg-canvas/60 p-3">
            <div className="flex flex-wrap items-center justify-between gap-3"><label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" aria-label={`${t(label)} · ${t(text)}`} checked={!!item} disabled={disabled} onChange={event => update(event.target.checked ? [...items, { modality, template: defaultTemplate(modality) }] : items.filter(meter => meter.modality !== modality))} />{t(text)}</label>{item && <select aria-label={`${t(label)} · ${t(text)} · ${t("计量方式")}`} className="min-h-11 rounded-control border border-hairline bg-canvas-raised px-3 text-sm" value={item.template} disabled={disabled} onChange={event => update(items.map((meter, i) => i === index ? { modality, template: event.target.value } : meter))}>{templates[modality].map(([key, name]) => <option key={key} value={key}>{t(name)}</option>)}</select>}</div>
            {item && (modality === "image" || modality === "video") && item.template !== "tokens" && <details className="mt-2 text-xs text-ink-secondary"><summary className="w-fit cursor-pointer py-2">{t("按尺寸或质量细分（可选）")}</summary><div className="mt-2">{variants(direction, index, item.variants || [], next => update(items.map((meter, i) => i === index ? { ...meter, variants: next as JSONValue[] } : meter)))}</div></details>}
          </div>;
        })}</div>}
        {direction === "input" && items.some(item => item.template === "tokens") && <p className="mt-3 text-xs leading-6 text-ink-mute">{t("Token 输入会自动列出普通输入和缓存读取两个单价；缓存写入归普通输入，不重复计费。")}</p>}
      </fieldset>;
    })}
  </section>;
}
