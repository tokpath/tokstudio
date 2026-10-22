"use client";
import { useI18n } from "@/components/i18n";
import { FieldHelp } from "./field-help";
import { businessFieldHelp } from "./business-field-help";
import { YesNoChoice } from "./yes-no";

import { useDeferredValue, useId, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { type JSONValue } from "@/lib/contracts";
import { request } from "@/lib/api-client";
import { ErrorPanel } from "./shared";
import { configDefaults, nodeVisible, retainKnownFields, supplierQuoteLines, type ConfigNode } from "./operation-config";

const control = "min-h-11 w-full min-w-0 rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand";
const asObject = (value: JSONValue | undefined): Record<string, JSONValue> => value && typeof value === "object" && !Array.isArray(value) ? value : {};

export function ConfigurationEditor({ schema, value, disabled, change, operatorID }: { schema: ConfigNode; value: string; disabled: boolean; change: (value: string) => void; operatorID?: string }) {
  const { t } = useI18n();
  const [emptyDefaults] = useState(() => configDefaults(schema) || {});
  let parsed: JSONValue;
  let invalid = false;
  try { parsed = value ? JSON.parse(value) as JSONValue : emptyDefaults; if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("configuration object required"); }
  catch { parsed = {}; invalid = true; }
  return <div className="space-y-5">
    {invalid ? <div className="rounded-control border border-danger/30 p-4 text-sm text-danger">{t("配置内容无法读取，请关闭后重新打开。")}</div> : <NodeEditor node={schema} value={parsed} disabled={disabled} operatorID={operatorID} change={next => change(JSON.stringify(retainKnownFields(next, schema)))} root />}
  </div>;
}

function NodeEditor({ node, value, change, disabled, context = "", root = false, fieldPath = "", operatorID, choose, referenceContext }: { node: ConfigNode; value: JSONValue | undefined; change: (value: JSONValue) => void; disabled: boolean; context?: string; root?: boolean; fieldPath?: string; operatorID?: string; choose?: (value: string, defaults: Record<string, unknown>) => void; referenceContext?: Record<string, JSONValue> }) {
  const { t } = useI18n();
  const id = useId();
  const help = businessFieldHelp(fieldPath, node.help, node.lookup);
  const label = context ? t("{label}（{context}）", { label: t(node.label), context }) : t(node.label);
  if (node.kind === "boolean") return <div data-config-path={fieldPath} className="min-w-0 text-sm"><span id={id} className="text-ink-secondary">{t(node.label)}</span>{node.required && <span className="ml-1 text-danger" aria-hidden="true">*</span>}{help && <FieldHelp title={t(node.label)}>{t(help)}</FieldHelp>}<YesNoChoice labelledBy={id} name={fieldPath || node.label} value={value === true} disabled={disabled} onChange={change} /></div>;
  if (node.kind === "object") {
    if (node.presentation === "capability") return <CapabilityFields node={node} value={value} change={change} disabled={disabled} operatorID={operatorID} context={context} fieldPath={fieldPath} />;
    const data = asObject(value);
    const update = (key: string, selected: JSONValue, defaults?: Record<string, unknown>) => {
      const next = { ...data, [key]: selected };
      if ((data.action === "listing" || data.action === "authorization") && (key === "action" || key === "buyer_id")) { next.model_id = ""; next.expected_version = ""; }
      if (key === "kind" && node.fields?.seller_id?.readOnly && node.fields?.buyer_id?.lookupBy) { next.buyer_id="";next.seller_id=selected==="supplier"?"":node.fields.seller_id.default || ""; }
      if (key === "buyer_id" && data.kind === "supplier" && typeof defaults?.provider_id === "string") next.seller_id=defaults.provider_id;
      if (key === "credential_id" && typeof defaults?.provider_id === "string" && defaults.provider_id.trim()) next.provider_id = defaults.provider_id;
      if (key === "model_id" && (data.action === "listing" || data.action === "authorization")) {
        const version = defaults?.[data.action === "authorization" ? "authorization_version" : "listing_version"];
        next.expected_version = typeof version === "number" && Number.isSafeInteger(version) && version >= 0 ? version : "";
      }
      change(next);
    };
    const children = <div className="grid gap-4 sm:grid-cols-2">{Object.entries(node.fields || {}).filter(([key, child]) => !child.hidden && !child.readOnly && !child.generated && key !== "version" && key !== "expected_version" && !key.endsWith("_version") && nodeVisible(child, data)).map(([key, child]) => <div key={key} className={["object", "array", "record", "textarea"].includes(child.kind) ? "min-w-0 sm:col-span-2" : "min-w-0"}><NodeEditor
      node={child.lookupBy ? {...child,lookup:child.lookupBy.values[String(data[child.lookupBy.field])]} : child.lookupWhen && !child.lookupWhen.values.includes(String(data[child.lookupWhen.field])) ? { ...child, lookup: undefined } : child}
      value={data[key]} disabled={disabled || !!child.readOnly} operatorID={operatorID} referenceContext={data}
      context={root ? context : [context, t(node.label)].filter(Boolean).join(" · ")} fieldPath={fieldPath ? `${fieldPath}.${key}` : key}
      change={next => update(key, next)} choose={(selected, defaults) => update(key, selected, defaults)}
    /></div>)}</div>;
    return root ? children : <fieldset className="min-w-0 rounded-control border border-hairline bg-canvas/40 p-4"><legend className="px-2 text-sm font-semibold">{t(node.label)}{help && <FieldHelp title={t(node.label)}>{t(help)}</FieldHelp>}</legend>{children}</fieldset>;
  }
  if (node.kind === "array" && node.presentation === "operations") {
    const items = Array.isArray(value) ? value : [];
    const selected = new Set(items.map(item => typeof item === "string" ? item : String(asObject(item).operation || "")));
    const choose = (key: string, checked: boolean) => change((node.options || []).filter(([op]) => op === key ? checked : selected.has(op)).map(([op]) => ({ operation: op })));
    return <fieldset data-config-path={fieldPath} className="min-w-0 text-sm sm:col-span-2"><legend className="text-sm font-medium">{label}{help && <FieldHelp title={label}>{t(help)}</FieldHelp>}</legend><div className="mt-2 grid gap-2 sm:grid-cols-2">{(node.options || []).map(([key, text]) => <label key={key} className="flex min-h-11 items-center gap-3 rounded-control border border-hairline px-3 py-2"><input type="checkbox" aria-label={t(text)} checked={selected.has(key)} disabled={disabled} onChange={event => choose(key, event.target.checked)} />{t(text)}</label>)}</div></fieldset>;
  }
  if (node.kind === "array") {
    const items = Array.isArray(value) ? value : [];
    if (node.options) {
      const known = node.options.map(([key]) => key);
      const selected = new Set(items.map(item => String(item)));
      const choose = (key: string, checked: boolean) => {
        const next = new Set(selected);
        if (checked) next.add(key); else next.delete(key);
        change(known.filter(item => next.has(item)));
      };
      return <fieldset data-config-path={fieldPath} className="min-w-0 text-sm"><legend className="text-sm font-medium">{t(node.label)}{help && <FieldHelp title={t(node.label)}>{t(help)}</FieldHelp>}</legend><div className="mt-2 grid gap-2 sm:grid-cols-2">{node.options.map(([key, text]) => <label key={key} className="flex min-h-11 items-center gap-3 rounded-control border border-hairline px-3 py-2"><input type="checkbox" aria-label={t(text)} checked={selected.has(key)} disabled={disabled} onChange={event => choose(key, event.target.checked)} />{t(text)}</label>)}</div></fieldset>;
    }
    return <section data-config-path={fieldPath} aria-label={label} className="min-w-0 space-y-3"><div className="flex flex-wrap items-center justify-between gap-3"><h4 className="text-sm font-medium">{t(node.label)}{help && <FieldHelp title={t(node.label)}>{t(help)}</FieldHelp>} <span className="font-normal text-ink-mute">{t("{count} 项", { count: items.length })}</span></h4><Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => change([...items, configDefaults(node.item!) ?? ""])}><Plus aria-hidden="true" />{t("添加{item}", { item: node.item?.kind === "object" ? t(node.item.label) : t("项目") })}</Button></div>{!items.length && <p className="rounded-control border border-dashed border-hairline p-4 text-xs text-ink-mute">{t("尚未添加{item}", { item: t(node.label) })}</p>}{items.map((item, index) => <div key={index} className="flex min-w-0 items-start gap-2"><div className="min-w-0 flex-1"><NodeEditor node={node.item!} value={item} disabled={disabled} operatorID={operatorID} context={`${t(node.label)} ${index + 1}`} fieldPath={`${fieldPath}.${index}`} change={next => change(items.map((previous, i) => i === index ? next : previous))} /></div><Button type="button" variant="ghost" size="icon" aria-label={t("移除{item} {index}", { item: t(node.label), index: index + 1 })} disabled={disabled} onClick={() => change(items.filter((_, i) => i !== index))}><Trash2 aria-hidden="true" /></Button></div>)}</section>;
  }
  if (node.kind === "record") {
    const entries = Object.entries(asObject(value));
    const update = (index: number, key: string, item: JSONValue) => change(Object.fromEntries(entries.map((entry, i) => i === index ? [key, item] : entry)));
    return <section data-config-path={fieldPath} aria-label={label} className="min-w-0 space-y-3"><div className="flex flex-wrap items-center justify-between gap-3"><h4 className="text-sm font-medium">{t(node.label)}{help && <FieldHelp title={t(node.label)}>{t(help)}</FieldHelp>}</h4><Button type="button" variant="outline" size="sm" disabled={disabled || entries.some(([key]) => !key)} onClick={() => change({ ...asObject(value), "": configDefaults(node.item!) ?? "" })}><Plus aria-hidden="true" />{t("添加{item}项目", { item: t(node.label) })}</Button></div>{entries.map(([key, item], index) => <div key={index} className="min-w-0 space-y-3 rounded-control border border-hairline p-3"><div className="flex items-end gap-2"><label className="block min-w-0 flex-1 text-xs text-ink-secondary">{t("项目名称")}<Input aria-label={t("{item}项目名称 {index}", { item: t(node.label), index: index + 1 })} className="mt-2" value={key} disabled={disabled} onChange={event => { const next = event.target.value; if (next === key || !entries.some(([existing]) => existing === next)) update(index, next, item); }} /></label><Button type="button" variant="ghost" size="icon" aria-label={t("移除{item}项目 {index}", { item: t(node.label), index: index + 1 })} disabled={disabled} onClick={() => change(Object.fromEntries(entries.filter((_, i) => i !== index)))}><Trash2 aria-hidden="true" /></Button></div><NodeEditor node={node.item!} value={item} disabled={disabled} operatorID={operatorID} context={`${t(node.label)} · ${key || index + 1}`} fieldPath={`${fieldPath}.${key}`} change={next => update(index, key, next)} /></div>)}</section>;
  }
  const raw = value === undefined || value === null ? "" : String(value);
  if (node.lookup && operatorID) return <div data-config-path={fieldPath}><ConfigReference node={{ ...node, help }} label={label} value={raw} operatorID={operatorID} disabled={disabled} referenceContext={referenceContext} change={(selected, defaults) => choose ? choose(selected, defaults) : change(selected)} /></div>;
  let displayed = raw;
  if (node.kind === "datetime" && raw) { const date = new Date(raw); if (!Number.isNaN(date.getTime())) displayed = new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16); }
  return <div data-config-path={fieldPath} className="min-w-0 text-sm"><label htmlFor={id} className="inline text-ink-secondary">{t(node.label)}</label>{node.required && <span className="ml-1 text-danger" aria-hidden="true">*</span>}{help && <FieldHelp title={t(node.label)}>{t(help)}</FieldHelp>}
    {node.kind === "select" ? <select id={id} aria-label={label} className={`${control} mt-2`} value={raw} disabled={disabled} onChange={event => change(event.target.value)}><option value="">{t("请选择")}</option>{node.options?.map(([key, text]) => <option key={key} value={key}>{t(text)}</option>)}</select>
        : node.kind === "textarea" ? <textarea id={id} aria-label={label} className={`${control} mt-2 min-h-24`} value={raw} disabled={disabled} onChange={event => change(event.target.value)} />
          : <Input id={id} aria-label={label} className="mt-2" type={node.kind === "datetime" ? "datetime-local" : "text"} inputMode={node.kind === "integer" ? "numeric" : node.kind === "decimal" ? "decimal" : undefined} value={displayed} disabled={disabled} onChange={event => { const next = event.target.value; change(node.kind === "integer" && (node.signed ? /^-?\d+$/ : /^\d+$/).test(next) && Number.isSafeInteger(Number(next)) ? Number(next) : next); }} />}

  </div>;
}

function CapabilityFields({ node, value, change, disabled, operatorID, context, fieldPath }: { node: ConfigNode; value: JSONValue | undefined; change: (value: JSONValue) => void; disabled: boolean; operatorID?: string; context?: string; fieldPath?: string }) {
  const { t } = useI18n();
  const data = asObject(value);
  const fields = node.fields || {};
  const quotes = Array.isArray(data.quotes) ? data.quotes.map(asObject) : [];
  const lines = supplierQuoteLines(String(data.operation || ""));
  const write = (next: Record<string, JSONValue>) => {
    const operation = String(next.operation || "");
    const current = Array.isArray(next.quotes) ? next.quotes.map(asObject) : quotes;
    const kept = supplierQuoteLines(operation).map(line => {
      const quote = current.find(item => String(item.key || "") === line.key);
      return { key: line.key, rate: String(quote?.rate || ""), bound: String(quote?.bound || "") };
    }).filter(quote => quote.rate || quote.bound);
    change({ ...next, quotes: kept });
  };
  const update = (key: string, selected: JSONValue) => write({ ...data, [key]: selected });
  const render = (key: string) => fields[key] && !fields[key].hidden ? <NodeEditor key={key} node={fields[key]} value={data[key]} disabled={disabled} operatorID={operatorID} fieldPath={fieldPath ? `${fieldPath}.${key}` : key} change={next => update(key, next)} /> : null;
  const setQuote = (key: string, patch: { rate?: string; bound?: string }) => {
    const current = lines.map(line => {
      const quote = quotes.find(item => String(item.key || "") === line.key);
      return { key: line.key, rate: String(quote?.rate || ""), bound: String(quote?.bound || "") };
    });
    write({ ...data, quotes: current.map(quote => quote.key === key ? { ...quote, ...patch } : quote) });
  };
  return <fieldset className="min-w-0 space-y-5 rounded-control border border-hairline bg-canvas/40 p-4"><legend className="px-2 text-sm font-semibold">{context || t(node.label)}{node.help && <FieldHelp title={t(node.label)}>{t(node.help)}</FieldHelp>}</legend>
    <section className="space-y-3"><div><h4 className="text-sm font-semibold">{t("怎么调用")}</h4><p className="mt-1 text-xs leading-6 text-ink-mute">{t("选择调用方式和转发协议。客户请求会原样转发。")}</p></div><div className="grid gap-4 sm:grid-cols-2">{["operation", "streaming"].map(key => <div key={key} className="min-w-0">{render(key)}</div>)}</div></section>
    <section className="space-y-3"><div><h4 className="text-sm font-semibold">{t("供应商报价")}</h4><p className="mt-1 text-xs leading-6 text-ink-mute">{t("这是模型提供商的报价，只对当前这种能力生效。文本按输入、输出和缓存命中计价；图片按张，语音按字符，转写和视频按秒。缓存命中可以留空，留空时输入按全部 token 计价。最大用量用于客户没有写明数量时占用额度。")}</p></div>
      {!lines.length && <p className="rounded-control border border-dashed border-hairline p-4 text-xs text-ink-mute">{t("先选择调用能力，再填写这种能力的报价。")}</p>}
      {lines.map(line => {
        const quote = quotes.find(item => String(item.key || "") === line.key);
        const rate = String(quote?.rate || "");
        const bound = String(quote?.bound || "");
        return <div key={line.key} className="grid gap-3 rounded-control border border-hairline p-3 sm:grid-cols-2"><label className="block text-xs text-ink-secondary">{t(line.label)}{t("单价")}<span className="mt-2 block text-sm text-ink">{t(line.perLabel)}</span><Input aria-label={t("{label}单价", { label: t(line.label) })} className="mt-2" inputMode="decimal" value={rate} disabled={disabled} placeholder={line.optional ? t("可以留空") : ""} onChange={event => setQuote(line.key, { rate: event.target.value })} /></label><label className="block text-xs text-ink-secondary">{t("最大用量")}<Input aria-label={t("{label}最大用量", { label: t(line.label) })} className="mt-2" value={bound} disabled={disabled} onChange={event => setQuote(line.key, { bound: event.target.value })} /></label></div>;
      })}
    </section>
  </fieldset>;
}

const ReferenceSchema = z.object({ items: z.array(z.object({ value: z.string(), label: z.string(), defaults: z.record(z.unknown()).optional() })), next_cursor: z.string().optional() });
function ConfigReference({ node, label, value, operatorID, disabled, change, referenceContext }: { node: ConfigNode; label: string; value: string; operatorID: string; disabled: boolean; change: (value: string, defaults: Record<string, unknown>) => void; referenceContext?: Record<string, JSONValue> }) {
  const { t } = useI18n();
  const [search, setSearch] = useState(""); const deferred = useDeferredValue(search); const [cursor, setCursor] = useState(""); const [selectedLabel, setSelectedLabel] = useState("");
  const path = `/api/v1/operators/${encodeURIComponent(operatorID)}/references/${node.lookup}`;
  const needsBuyer = node.lookup === "models" && referenceContext?.action === "authorization", buyerID = needsBuyer ? String(referenceContext?.buyer_id || "") : "";
  const ready = !needsBuyer || !!buyerID;
  const data = useQuery({ queryKey: ["references", path, deferred, cursor, buyerID], enabled: ready, queryFn: ({ signal }) => request(`${path}?q=${encodeURIComponent(deferred)}&cursor=${encodeURIComponent(cursor)}${needsBuyer ? `&buyer_id=${encodeURIComponent(buyerID)}` : ""}`, ReferenceSchema, { signal }) });
  const selected = useQuery({ queryKey: ["reference-label", path, value, buyerID], enabled: ready && !!value && value !== "*" && !data.data?.items.some(item => item.value === value), queryFn: ({ signal }) => request(`${path}?q=${encodeURIComponent(value)}${needsBuyer ? `&buyer_id=${encodeURIComponent(buyerID)}` : ""}`, ReferenceSchema, { signal }) });
  const currentLabel = data.data?.items.find(item => item.value === value)?.label || selected.data?.items.find(item => item.value === value)?.label || selectedLabel;
  return <fieldset className="min-w-0 text-sm"><legend className="text-ink-secondary">{t(node.label)}{node.required && <span className="ml-1 text-danger" aria-hidden="true">*</span>}{node.help && <FieldHelp title={t(node.label)}>{t(node.help)}</FieldHelp>}</legend><Input aria-label={t("搜索{label}", { label })} className="mt-2" value={search} disabled={disabled || !ready} placeholder={t("按名称或业务编号搜索")} onChange={event => { setSearch(event.target.value); setCursor(""); }} /><select aria-label={label} className={`${control} mt-2`} value={value} disabled={disabled || !ready || data.isPending} onChange={event => { const selected = data.data?.items.find(item => item.value === event.target.value); setSelectedLabel(selected?.label || ""); change(event.target.value, selected?.defaults || {}); }}><option value="">{ready ? t("请选择实际业务记录") : t("请先选择被授权的直属下级")}</option>{node.wildcard && <option value="*">{t("全部模型的默认跟随规则（*）")}</option>}{value && value !== "*" && !data.data?.items.some(item => item.value === value) && <option value={value}>{currentLabel || t(selected.isPending ? "正在读取关联记录名称…" : "已选择关联记录")}</option>}{data.data?.items.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}</select>{data.isError && <div className="mt-2"><ErrorPanel compact error={data.error} retry={() => void data.refetch()} /></div>}{data.data?.next_cursor && <Button type="button" size="sm" variant="outline" disabled={disabled} className="mt-2" onClick={() => setCursor(data.data!.next_cursor!)}>{t("更多记录")}</Button>}</fieldset>;
}
