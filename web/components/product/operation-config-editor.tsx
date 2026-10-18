"use client";

import { useDeferredValue, useId, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { type JSONValue } from "@/lib/contracts";
import { request } from "@/lib/api-client";
import { ErrorPanel } from "./shared";
import { configDefaults, nodeVisible, type ConfigNode } from "./operation-config";

const control = "min-h-11 w-full min-w-0 rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand";
const asObject = (value: JSONValue | undefined): Record<string, JSONValue> => value && typeof value === "object" && !Array.isArray(value) ? value : {};

export function ConfigurationEditor({ schema, value, disabled, change, operatorID }: { schema: ConfigNode; value: string; disabled: boolean; change: (value: string) => void; operatorID?: string }) {
  let parsed: JSONValue;
  let invalid = false;
  try { parsed = value ? JSON.parse(value) as JSONValue : configDefaults(schema) || {}; if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("configuration object required"); }
  catch { parsed = {}; invalid = true; }
  return <div className="space-y-5">
    {invalid ? <div className="rounded-control border border-danger/30 p-4 text-sm text-danger">高级配置内容无法读取，请展开技术高级配置并修正格式。</div> : <NodeEditor node={schema} value={parsed} disabled={disabled} operatorID={operatorID} change={next => change(JSON.stringify(next))} root />}
    <details className="rounded-control border border-hairline bg-canvas p-4 text-sm">
      <summary className="cursor-pointer font-medium text-ink-secondary">技术高级配置</summary>
      <p className="my-3 text-xs leading-6 text-ink-mute">供技术人员导入完整验证配置或维护扩展字段。结构化编辑会保留额外字段；此处修改后请重新核对所有业务条款。</p>
      <textarea aria-label="完整技术配置 JSON" className={`${control} min-h-64 font-mono text-xs`} value={value} disabled={disabled} onChange={event => change(event.target.value)} spellCheck={false} />
    </details>
  </div>;
}

function NodeEditor({ node, value, change, disabled, context = "", root = false, fieldPath = "", operatorID, choose, referenceContext }: { node: ConfigNode; value: JSONValue | undefined; change: (value: JSONValue) => void; disabled: boolean; context?: string; root?: boolean; fieldPath?: string; operatorID?: string; choose?: (value: string, defaults: Record<string, unknown>) => void; referenceContext?: Record<string, JSONValue> }) {
  const id = useId();
  const label = `${node.label}${context ? `（${context}）` : ""}`;
  if (node.kind === "object") {
    const data = asObject(value);
    const update = (key: string, selected: JSONValue, defaults?: Record<string, unknown>) => {
      const next = { ...data, [key]: selected };
      if ((data.action === "listing" || data.action === "authorization") && (key === "action" || key === "buyer_id")) { next.model_id = ""; next.expected_version = ""; }
      if (key === "credential_id" && typeof defaults?.provider_id === "string" && defaults.provider_id.trim()) next.provider_id = defaults.provider_id;
      if (key === "public_model_id" && typeof defaults?.identity === "string" && defaults.identity.trim()) next.identity = defaults.identity;
      if (key === "model_id" && (data.action === "listing" || data.action === "authorization")) {
        const version = defaults?.[data.action === "authorization" ? "authorization_version" : "listing_version"];
        next.expected_version = typeof version === "number" && Number.isSafeInteger(version) && version >= 0 ? version : "";
      }
      change(next);
    };
    const children = <div className="grid gap-4 sm:grid-cols-2">{Object.entries(node.fields || {}).filter(([, child]) => nodeVisible(child, data)).map(([key, child]) => <div key={key} className={["object", "array", "record", "textarea"].includes(child.kind) ? "min-w-0 sm:col-span-2" : "min-w-0"}><NodeEditor
      node={child.lookupWhen && !child.lookupWhen.values.includes(String(data[child.lookupWhen.field])) ? { ...child, lookup: undefined } : child}
      value={data[key]} disabled={disabled || !!child.readOnly} operatorID={operatorID} referenceContext={data}
      context={root ? context : [context, node.label].filter(Boolean).join(" · ")} fieldPath={fieldPath ? `${fieldPath}.${key}` : key}
      change={next => update(key, next)} choose={(selected, defaults) => update(key, selected, defaults)}
    /></div>)}</div>;
    return root ? children : <fieldset className="min-w-0 rounded-control border border-hairline bg-canvas/40 p-4"><legend className="px-2 text-sm font-semibold">{node.label}</legend>{node.help && <p className="mb-4 text-xs leading-6 text-ink-mute">{node.help}</p>}{children}</fieldset>;
  }
  if (node.kind === "array") {
    const items = Array.isArray(value) ? value : [];
    return <section data-config-path={fieldPath} aria-label={label} className="min-w-0 space-y-3"><div className="flex flex-wrap items-center justify-between gap-3"><h4 className="text-sm font-medium">{node.label} <span className="font-normal text-ink-mute">{items.length} 项</span></h4><Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => change([...items, configDefaults(node.item!) ?? ""])}><Plus aria-hidden="true" />添加{node.item?.kind === "object" ? node.item.label : "项目"}</Button></div>{node.help && <p className="text-xs leading-6 text-ink-mute">{node.help}</p>}{!items.length && <p className="rounded-control border border-dashed border-hairline p-4 text-xs text-ink-mute">尚未添加{node.label}</p>}{items.map((item, index) => <div key={index} className="flex min-w-0 items-start gap-2"><div className="min-w-0 flex-1"><NodeEditor node={node.item!} value={item} disabled={disabled} operatorID={operatorID} context={`${node.label} ${index + 1}`} fieldPath={`${fieldPath}.${index}`} change={next => change(items.map((previous, i) => i === index ? next : previous))} /></div><Button type="button" variant="ghost" size="icon" aria-label={`移除${node.label} ${index + 1}`} disabled={disabled} onClick={() => change(items.filter((_, i) => i !== index))}><Trash2 aria-hidden="true" /></Button></div>)}</section>;
  }
  if (node.kind === "record") {
    const entries = Object.entries(asObject(value));
    const update = (index: number, key: string, item: JSONValue) => change(Object.fromEntries(entries.map((entry, i) => i === index ? [key, item] : entry)));
    return <section data-config-path={fieldPath} aria-label={label} className="min-w-0 space-y-3"><div className="flex flex-wrap items-center justify-between gap-3"><h4 className="text-sm font-medium">{node.label}</h4><Button type="button" variant="outline" size="sm" disabled={disabled || entries.some(([key]) => !key)} onClick={() => change({ ...asObject(value), "": configDefaults(node.item!) ?? "" })}><Plus aria-hidden="true" />添加{node.label}项目</Button></div>{node.help && <p className="text-xs leading-6 text-ink-mute">{node.help}</p>}{entries.map(([key, item], index) => <div key={index} className="min-w-0 space-y-3 rounded-control border border-hairline p-3"><div className="flex items-end gap-2"><label className="block min-w-0 flex-1 text-xs text-ink-secondary">项目名称<Input aria-label={`${node.label}项目名称 ${index + 1}`} className="mt-2" value={key} disabled={disabled} onChange={event => { const next = event.target.value; if (next === key || !entries.some(([existing]) => existing === next)) update(index, next, item); }} /></label><Button type="button" variant="ghost" size="icon" aria-label={`移除${node.label}项目 ${index + 1}`} disabled={disabled} onClick={() => change(Object.fromEntries(entries.filter((_, i) => i !== index)))}><Trash2 aria-hidden="true" /></Button></div><NodeEditor node={node.item!} value={item} disabled={disabled} operatorID={operatorID} context={`${node.label} · ${key || index + 1}`} fieldPath={`${fieldPath}.${key}`} change={next => update(index, key, next)} /></div>)}</section>;
  }
  const raw = value === undefined || value === null ? "" : String(value);
  if (node.lookup && operatorID) return <div data-config-path={fieldPath}><ConfigReference node={node} label={label} value={raw} operatorID={operatorID} disabled={disabled} referenceContext={referenceContext} change={(selected, defaults) => choose ? choose(selected, defaults) : change(selected)} /></div>;
  let displayed = raw;
  if (node.kind === "datetime" && raw) { const date = new Date(raw); if (!Number.isNaN(date.getTime())) displayed = new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16); }
  return <div data-config-path={fieldPath} className="min-w-0 text-sm"><label htmlFor={id} className="block text-ink-secondary">{node.label}{node.required && <span className="ml-1 text-danger">*</span>}</label>
    {node.kind === "boolean" ? <label className="mt-2 flex min-h-11 items-center gap-3 rounded-control border border-hairline px-3 py-2"><input id={id} aria-label={label} type="checkbox" checked={value === true} disabled={disabled} onChange={event => change(event.target.checked)} />{value === true ? "是" : "否"}</label>
      : node.kind === "select" ? <select id={id} aria-label={label} className={`${control} mt-2`} value={raw} disabled={disabled} onChange={event => change(event.target.value)}><option value="">请选择</option>{node.options?.map(([key, text]) => <option key={key} value={key}>{text}</option>)}</select>
        : node.kind === "textarea" ? <textarea id={id} aria-label={label} className={`${control} mt-2 min-h-24`} value={raw} disabled={disabled} onChange={event => change(event.target.value)} />
          : <Input id={id} aria-label={label} className="mt-2" type={node.kind === "datetime" ? "datetime-local" : "text"} inputMode={node.kind === "integer" ? "numeric" : node.kind === "decimal" ? "decimal" : undefined} value={displayed} disabled={disabled} onChange={event => { const next = event.target.value; change(node.kind === "integer" && (node.signed ? /^-?\d+$/ : /^\d+$/).test(next) && Number.isSafeInteger(Number(next)) ? Number(next) : next); }} />}
    {node.help && <p className="mt-2 text-xs leading-6 text-ink-mute">{node.help}</p>}
  </div>;
}

const ReferenceSchema = z.object({ items: z.array(z.object({ value: z.string(), label: z.string(), defaults: z.record(z.unknown()).optional() })), next_cursor: z.string().optional() });
function ConfigReference({ node, label, value, operatorID, disabled, change, referenceContext }: { node: ConfigNode; label: string; value: string; operatorID: string; disabled: boolean; change: (value: string, defaults: Record<string, unknown>) => void; referenceContext?: Record<string, JSONValue> }) {
  const [search, setSearch] = useState(""); const deferred = useDeferredValue(search); const [cursor, setCursor] = useState(""); const [selectedLabel, setSelectedLabel] = useState("");
  const path = `/api/v1/operators/${encodeURIComponent(operatorID)}/references/${node.lookup}`;
  const needsBuyer = node.lookup === "models" && referenceContext?.action === "authorization", buyerID = needsBuyer ? String(referenceContext?.buyer_id || "") : "";
  const ready = !needsBuyer || !!buyerID;
  const data = useQuery({ queryKey: ["references", path, deferred, cursor, buyerID], enabled: ready, queryFn: ({ signal }) => request(`${path}?q=${encodeURIComponent(deferred)}&cursor=${encodeURIComponent(cursor)}${needsBuyer ? `&buyer_id=${encodeURIComponent(buyerID)}` : ""}`, ReferenceSchema, { signal }) });
  return <fieldset className="min-w-0 text-sm"><legend className="text-ink-secondary">{node.label}{node.required && <span className="ml-1 text-danger">*</span>}</legend><Input aria-label={`搜索${label}`} className="mt-2" value={search} disabled={disabled || !ready} placeholder="按名称或业务编号搜索" onChange={event => { setSearch(event.target.value); setCursor(""); }} /><select aria-label={label} className={`${control} mt-2`} value={value} disabled={disabled || !ready || data.isPending} onChange={event => { const selected = data.data?.items.find(item => item.value === event.target.value); setSelectedLabel(selected?.label || ""); change(event.target.value, selected?.defaults || {}); }}><option value="">{ready ? "请选择实际业务记录" : "请先选择被授权的直属下级"}</option>{node.wildcard && <option value="*">全部模型的默认跟随规则（*）</option>}{value && value !== "*" && !data.data?.items.some(item => item.value === value) && <option value={value}>{selectedLabel || value}</option>}{data.data?.items.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}</select>{data.isError && <div className="mt-2"><ErrorPanel compact error={data.error} retry={() => void data.refetch()} /></div>}{data.data?.next_cursor && <Button type="button" size="sm" variant="outline" disabled={disabled} className="mt-2" onClick={() => setCursor(data.data!.next_cursor!)}>更多记录</Button>}{node.help && <p className="mt-2 text-xs leading-6 text-ink-mute">{node.help}</p>}</fieldset>;
}
