"use client";
import { useI18n } from "@/components/i18n";
import { FieldHelp } from "./field-help";
import { businessFieldHelp } from "./business-field-help";
import { YesNoChoice } from "./yes-no";

import { useDeferredValue, useId, useState, type ReactNode } from "react";
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
  const { t } = useI18n();
  const [emptyDefaults] = useState(() => configDefaults(schema) || {});
  let parsed: JSONValue;
  let invalid = false;
  try { parsed = value ? JSON.parse(value) as JSONValue : emptyDefaults; if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("configuration object required"); }
  catch { parsed = {}; invalid = true; }
  return <div className="space-y-5">
    {invalid ? <div className="rounded-control border border-danger/30 p-4 text-sm text-danger">{t("高级配置内容无法读取，请展开技术高级配置并修正格式。")}</div> : <NodeEditor node={schema} value={parsed} disabled={disabled} operatorID={operatorID} change={next => change(JSON.stringify(next))} root />}
    <details className="rounded-control border border-hairline bg-canvas p-4 text-sm">
      <summary className="cursor-pointer font-medium text-ink-secondary">{t("技术高级配置")}</summary>
      <p className="my-3 text-xs leading-6 text-ink-mute">{t("供技术人员导入完整验证配置或维护扩展字段。结构化编辑会保留额外字段；此处修改后请重新核对所有业务条款。")}</p>
      <textarea aria-label={t("完整技术配置 JSON")} className={`${control} min-h-64 font-mono text-xs`} value={value} disabled={disabled} onChange={event => change(event.target.value)} spellCheck={false} />
    </details>
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
      if (key === "public_model_id" && typeof defaults?.identity === "string" && defaults.identity.trim()) next.identity = defaults.identity;
      if (key === "model_id" && (data.action === "listing" || data.action === "authorization")) {
        const version = defaults?.[data.action === "authorization" ? "authorization_version" : "listing_version"];
        next.expected_version = typeof version === "number" && Number.isSafeInteger(version) && version >= 0 ? version : "";
      }
      change(next);
    };
    const children = <div className="grid gap-4 sm:grid-cols-2">{Object.entries(node.fields || {}).filter(([key, child]) => !child.readOnly && !child.generated && key !== "version" && key !== "expected_version" && !key.endsWith("_version") && nodeVisible(child, data)).map(([key, child]) => <div key={key} className={["object", "array", "record", "textarea"].includes(child.kind) ? "min-w-0 sm:col-span-2" : "min-w-0"}><NodeEditor
      node={child.lookupBy ? {...child,lookup:child.lookupBy.values[String(data[child.lookupBy.field])]} : child.lookupWhen && !child.lookupWhen.values.includes(String(data[child.lookupWhen.field])) ? { ...child, lookup: undefined } : child}
      value={data[key]} disabled={disabled || !!child.readOnly} operatorID={operatorID} referenceContext={data}
      context={root ? context : [context, t(node.label)].filter(Boolean).join(" · ")} fieldPath={fieldPath ? `${fieldPath}.${key}` : key}
      change={next => update(key, next)} choose={(selected, defaults) => update(key, selected, defaults)}
    /></div>)}</div>;
    return root ? children : <fieldset className="min-w-0 rounded-control border border-hairline bg-canvas/40 p-4"><legend className="px-2 text-sm font-semibold">{t(node.label)}{help && <FieldHelp title={t(node.label)}>{t(help)}</FieldHelp>}</legend>{children}</fieldset>;
  }
  if (node.kind === "array") {
    const items = Array.isArray(value) ? value : [];
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

type MeterRow = { component: string; unit: string; path: string; zero: boolean; bound: string; extra: Record<string, JSONValue> };
function meterRows(data: Record<string, JSONValue>): MeterRow[] {
  const rules = Array.isArray(data.usage_rules) ? data.usage_rules : [];
  const bounds = asObject(data.billable_bounds);
  const seen = new Set<string>();
  const rows = rules.map(rule => {
    const item = asObject(rule);
    const component = String(item.component || "");
    seen.add(component);
    const extra = { ...item };
    for (const key of ["component", "unit", "path", "zero_when_absent", "bound"]) delete extra[key];
    const stored = component && bounds[component] !== undefined ? bounds[component] : item.bound;
    return { component, unit: String(item.unit || ""), path: String(item.path || ""), zero: item.zero_when_absent === true, bound: stored === undefined || stored === null ? "" : String(stored), extra };
  });
  for (const [key, bound] of Object.entries(bounds)) if (!seen.has(key)) rows.push({ component: key, unit: "", path: "", zero: false, bound: bound === undefined || bound === null ? "" : String(bound), extra: {} });
  return rows;
}
function writeMeter(data: Record<string, JSONValue>, rows: MeterRow[]): Record<string, JSONValue> {
  const bounds: Record<string, JSONValue> = {};
  const rules = rows.map(row => {
    const rule: Record<string, JSONValue> = { ...row.extra, component: row.component, unit: row.unit, path: row.path, zero_when_absent: row.zero };
    if (row.component.trim()) bounds[row.component] = row.bound;
    else if (row.bound) rule.bound = row.bound;
    return rule;
  });
  return { ...data, usage_rules: rules, billable_bounds: bounds };
}

function CapabilityFields({ node, value, change, disabled, operatorID, context, fieldPath }: { node: ConfigNode; value: JSONValue | undefined; change: (value: JSONValue) => void; disabled: boolean; operatorID?: string; context?: string; fieldPath?: string }) {
  const { t } = useI18n();
  const data = asObject(value);
  const fields = node.fields || {};
  const update = (key: string, selected: JSONValue) => change({ ...data, [key]: selected });
  const render = (key: string) => fields[key] ? <NodeEditor key={key} node={fields[key]} value={data[key]} disabled={disabled} operatorID={operatorID} fieldPath={fieldPath ? `${fieldPath}.${key}` : key} change={next => update(key, next)} /> : null;
  const rows = meterRows(data);
  const setRows = (next: MeterRow[]) => change(writeMeter(data, next));
  const advancedOpen = Number(data.max_output) > 0 || !!String(data.output_limit_parameter || "") || Object.keys(asObject(data.parameter_constraints)).length > 0 || Object.keys(asObject(data.subtract_paths)).length > 0;
  const section = (title: string, help: string, body: ReactNode) => <section className="space-y-3"><div><h4 className="text-sm font-semibold">{t(title)}</h4><p className="mt-1 text-xs leading-6 text-ink-mute">{t(help)}</p></div>{body}</section>;
  return <fieldset className="min-w-0 space-y-5 rounded-control border border-hairline bg-canvas/40 p-4"><legend className="px-2 text-sm font-semibold">{context || t(node.label)}{node.help && <FieldHelp title={t(node.label)}>{t(node.help)}</FieldHelp>}</legend>
    {section("怎么调用", "写清这项能力接受什么输入和参数，以及最大能接收多大的输入。", <div className="grid gap-4 sm:grid-cols-2">{["operation", "streaming", "input_types", "parameters", "required_parameters", "max_input_bytes"].map(key => <div key={key} className={fields[key] && ["object", "array", "record", "textarea"].includes(fields[key].kind) ? "min-w-0 sm:col-span-2" : "min-w-0"}>{render(key)}</div>)}</div>)}
    {section("怎么计费", "每一行是一项计费。名称以后要和价格里的组件一致，例如 input_tokens。用量路径是供应商返回用量里的位置，例如 /input_tokens。", <div className="space-y-3"><div className="flex justify-end"><Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => setRows([...rows, { component: "", unit: "", path: "", zero: false, bound: "", extra: {} }])}><Plus aria-hidden="true" />{t("添加计费项目")}</Button></div>{!rows.length && <p className="rounded-control border border-dashed border-hairline p-4 text-xs text-ink-mute">{t("尚未添加计费项目。对话常见的是输入和输出 token。")}</p>}{rows.map((row, index) => <div key={index} className="space-y-3 rounded-control border border-hairline p-3"><div className="grid gap-3 sm:grid-cols-2">{([["component", "计费名称"], ["unit", "计量单位"], ["path", "用量路径"], ["bound", "验证过的最大用量"]] as const).map(([key, label]) => <label key={key} className="block text-xs text-ink-secondary">{t(label)}<Input aria-label={t("{label} {index}", { label: t(label), index: index + 1 })} className="mt-2" value={row[key]} disabled={disabled} onChange={event => setRows(rows.map((item, i) => i === index ? { ...item, [key]: event.target.value } : item))} /></label>)}</div><div className="flex items-center justify-between gap-3"><label className="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" aria-label={t("没有这项用量时按 0 {index}", { index: index + 1 })} checked={row.zero} disabled={disabled} onChange={event => setRows(rows.map((item, i) => i === index ? { ...item, zero: event.target.checked } : item))} />{t("没有这项用量时按 0")}</label><Button type="button" variant="ghost" size="icon" aria-label={t("移除计费项目 {index}", { index: index + 1 })} disabled={disabled} onClick={() => setRows(rows.filter((_, i) => i !== index))}><Trash2 aria-hidden="true" /></Button></div></div>)}</div>)}
    {section("保存前确认", "四项都要勾上，并填写验证依据和时间。缺任何一项都无法保存这项能力。", <div className="grid gap-4 sm:grid-cols-2">{["evidence_id", "verified_at", "parameters_verified", "usage_verified", "bounds_verified", "recovery_verified"].map(key => <div key={key} className="min-w-0">{render(key)}</div>)}</div>)}
    <details className="rounded-control border border-hairline p-4" open={advancedOpen || undefined}><summary className="cursor-pointer text-sm font-medium">{t("特殊规则（一般不用填）")}</summary><p className="mb-4 mt-2 text-xs leading-6 text-ink-mute">{t("只有供应商限制了输出数量、参数取值范围，或有重复计算的用量时再填写。")}</p><div className="grid gap-4 sm:grid-cols-2">{["max_output", "output_limit_parameter", "parameter_constraints", "subtract_paths"].map(key => <div key={key} className={fields[key] && ["object", "array", "record", "textarea"].includes(fields[key].kind) ? "min-w-0 sm:col-span-2" : "min-w-0"}>{render(key)}</div>)}</div></details>
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
