"use client";

import { useQuery } from "@tanstack/react-query";
import { Check, CheckCircle2, LockKeyhole, ShieldCheck } from "lucide-react";
import { z } from "zod";
import { useDeferredValue, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { request } from "@/lib/api-client";
import { CommandResultSchema, Decimal, SignedDecimal, JSONValueSchema, type CommandField, type Entity, type Operation, type JSONValue } from "@/lib/contracts";
import { compareDecimal } from "@/lib/decimal";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { PaymentAction } from "./customer-plans";
import { ErrorPanel, Notice, useCommand } from "./shared";
import { configDefaults, configurationSchema, ConfigurationError, prepareConfiguration, validateModelConfiguration, validatePriceConfiguration } from "./operation-config";
import { ConfigurationEditor } from "./operation-config-editor";

function nested(value: unknown, path: string): unknown { return path.split(".").reduce<unknown>((current, key) => current !== null && typeof current === "object" ? (current as Record<string, unknown>)[key] : undefined, value); }
function assign(target: Record<string, JSONValue>, path: string, value: JSONValue) {
  const keys = path.split(".");
  if (keys.some(key => ["__proto__", "prototype", "constructor"].includes(key))) throw new Error("字段路径不合法");
  let current: Record<string, JSONValue> | JSONValue[] = target;
  keys.forEach((key, index) => { const object = current as Record<string, JSONValue>; if (index === keys.length - 1) { object[key] = value; return; } if (!object[key]) object[key] = /^\d+$/.test(keys[index + 1]) ? [] : {}; current = object[key] as Record<string, JSONValue>; });
}
function fieldString(field: CommandField, value: unknown): string {
  if (value === undefined || value === null) return field.kind === "boolean" ? "false" : "";
  if (field.kind === "csv" && Array.isArray(value)) return value.join(",");
  if (field.kind === "datetime" && typeof value === "string") { const date = new Date(value); if (!Number.isNaN(date.getTime())) return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16); }
  return typeof value === "object" ? JSON.stringify(value, null, 2) : String(value);
}
function secretField(field: CommandField): boolean { return field.kind === "password" || /secret|private_key|api_v3_key|credentials.*api_key/.test(field.name); }
function fieldName(field: CommandField): string { return field.name.split(".").at(-1)!; }
function primaryReference(operation: Operation, field: CommandField): boolean {
  const path = operation.path.split("?")[0];
  return (field.name === "id" && /\/(payment-methods|providers)$/.test(path)) || (field.name === "package_id" && /\/plans$/.test(path)) || (field.name === "campaign_id" && /\/(marketing|activities)$/.test(path));
}
function mergeValues(base: JSONValue, value: JSONValue): JSONValue {
  if (base && value && typeof base === "object" && typeof value === "object" && !Array.isArray(base) && !Array.isArray(value)) return Object.fromEntries(Object.entries({ ...base, ...value }).map(([key, next]) => [key, base[key] !== undefined && value[key] !== undefined ? mergeValues(base[key], value[key]) : next]));
  return value;
}
function initial(field: CommandField, operation: Operation, row?: Entity, context?: Record<string, JSONValue>, operatorID?: string): string {
  if (secretField(field)) return "";
  const supplied = context?.[field.name] ?? nested(context, field.name);
  if (supplied !== undefined) return fieldString(field, supplied);
  if (field.kind === "json") {
    const schema = configurationSchema(operation, operatorID);
    if (schema) {
      let source = nested(row?.values, field.name) ?? field.default_value;
      if (row && /\/catalog$/.test(operation.path)) source = { model: row.values, expected_version: row.version ?? 0 };
      if (row && /\/routes$/.test(operation.path)) source = { upstream: row.values, expected_version: row.version ?? 0 };
      const defaults = configDefaults(schema) || {};
      return JSON.stringify(source === undefined ? defaults : mergeValues(defaults, JSONValueSchema.parse(source)));
    }
  }
  if ((fieldName(field) === "expected_version" || fieldName(field) === "version") && row?.version !== undefined) return String(row.version);
  return fieldString(field, nested(row?.values, field.name) ?? field.default_value);
}
function visible(field: CommandField, values: Record<string, string>) { return field.kind !== "hidden" && (!field.visible_when || field.visible_when.values.includes(values[field.visible_when.field] || "")); }
function group(field: CommandField): string {
  if (field.name.startsWith("credentials.") || field.name === "api_base_url") return "通道接入材料";
  if (/evidence|reference|reason|explanation|terms/.test(field.name) && !field.lookup_url) return "依据与办理说明";
  if (/(_at$|^from$|^to$|timezone|period|seconds)/.test(field.name)) return "时间与有效期";
  if (field.kind === "decimal" || /budget|limit|approval|confirmation/.test(field.name)) return "金额、额度与办理规则";
  return "办理信息";
}
const groupHelp: Record<string, string> = {
  "办理信息": "选择实际业务记录，并填写本次要办理的事项。",
  "金额、额度与办理规则": "按字段标示的 USD、ATG 或原币种填写。比例使用 0 到 1 的小数；精确值按原样提交。",
  "时间与有效期": "时间按当前设备时区显示，提交时保存为统一时刻。",
  "依据与办理说明": "填写可追溯的原始凭证、规则或核实依据，便于后续复核。",
  "通道接入材料": "只展示所选通道需要的材料。启用前请确认材料与实际商户一致。",
};

type OperationFormProps = {
  operation: Operation;
  row?: Entity;
  operatorID?: string;
  initialValues?: Record<string, JSONValue>;
  lockedFields?: string[];
  embedded?: boolean;
  onBlockedChange?: (blocked: boolean) => void;
  onClose?: () => void;
};

export function OperationForm({ operation, row, operatorID, initialValues, lockedFields = [], embedded = false, onBlockedChange, onClose }: OperationFormProps) {
  const path = operation.path.replaceAll("{id}", encodeURIComponent(row?.id || "")).replaceAll("{operator_id}", encodeURIComponent(operatorID || ""));
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(operation.fields.map(field => [field.name, initial(field, operation, row, initialValues, operatorID)])));
  const [files, setFiles] = useState<Record<string, File>>({});
  const [confirmed, setConfirmed] = useState(false);
  const [validationError, setValidationError] = useState<string>();
  const [invalidField, setInvalidField] = useState<string>();
  const [invalidConfigPath, setInvalidConfigPath] = useState<string>();
  const formRef = useRef<HTMLFormElement>(null);
  const command = useCommand(path, CommandResultSchema, operation.method);
  const blocked = command.busy || command.unresolved;
  const disabled = blocked || !!command.result;
  const scoped = operatorID ? path.startsWith(`/api/v1/operators/${encodeURIComponent(operatorID)}/`) : path.startsWith("/api/v1/me/");
  const fields = operation.fields.filter(field => visible(field, values));
  const groups = Array.from(new Set(fields.map(group)));
  const schema = configurationSchema(operation, operatorID);
  const objectName = row && ["display_name", "name", "plan_name", "customer_name", "title"].map(key => row.values[key]).find(value => typeof value === "string" && value);

  useEffect(() => { onBlockedChange?.(blocked); }, [blocked, onBlockedChange]);
  useEffect(() => {
    if (!blocked) return;
    const guard = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ""; };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [blocked]);
  function focusField(name?: string, configPath?: string) {
    const configTarget = configPath && Array.from(formRef.current?.querySelectorAll<HTMLElement>("[data-config-path]") || []).find(element => element.dataset.configPath === configPath);
    const target = Array.from(formRef.current?.querySelectorAll<HTMLElement>("[data-command-field]") || []).find(element => element.dataset.commandField === name);
    const destination = configTarget || target;
    destination?.querySelector<HTMLElement>("input,select,textarea,button")?.focus();
    destination?.scrollIntoView?.({ behavior: "smooth", block: "center" });
  }
  function change(name: string, value: string) {
    if (disabled || lockedFields.includes(name)) return;
    setValues(previous => ({ ...previous, [name]: value }));
    setConfirmed(false);
    if (invalidField === name) { setInvalidField(undefined); setValidationError(undefined); }
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setValidationError(undefined); setInvalidField(undefined); setInvalidConfigPath(undefined);
    if (!scoped) { setValidationError("操作不属于当前经营空间，已阻止提交。"); return; }
    if (command.busy || command.result) return;
    let currentField: string | undefined;
    try {
      if (operation.confirmation && !confirmed) throw new Error("请阅读并确认本次办理影响后提交。");
      const payload: Record<string, JSONValue> = {};
      const activeFiles: Record<string, File> = {};
      for (const field of operation.fields) {
        if (field.visible_when && !field.visible_when.values.includes(values[field.visible_when.field] || "")) continue;
        currentField = field.name;
        const value = values[field.name] || "";
        if (field.kind === "file") { if (field.required && !files[field.name]) throw new Error(`请上传${field.label}`); if (files[field.name]) activeFiles[field.name] = files[field.name]; continue; }
        if (!value && !field.required) { if (field.kind === "csv") assign(payload, field.name, []); else if (field.name.startsWith("values.") || fieldName(field) === "second_review_above") assign(payload, field.name, null); continue; }
        if (!value && field.required) throw new Error(`请填写${field.label}`);
        let parsed: JSONValue;
        if (field.kind === "csv") parsed = value.split(",").map(item => item.trim()).filter(Boolean);
        else if (field.kind === "decimal") {
          const signed = /\/(risk|alerts)$/.test(operation.path) && ["warning", "critical"].includes(fieldName(field));
          if (!(signed ? SignedDecimal : Decimal).safeParse(value).success || (value.split(".")[1]?.length || 0) > 18) throw new Error(`${field.label}请输入${signed ? "" : "非负"}十进制数，最多 18 位小数`);
          if (["direct_rate", "indirect_rate", "concession_rate", "fee_rate", "maximum_ratio"].includes(fieldName(field)) && compareDecimal(value, "1") > 0) throw new Error(`${field.label}应使用 0 到 1 的精确小数，例如 5% 填写 0.05`);
          parsed = value;
        }
        else if (field.kind === "integer" || fieldName(field) === "expected_version" || fieldName(field) === "version") { if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) throw new Error(`${field.label}应为非负整数`); parsed = Number(value); }
        else if (field.kind === "boolean" || (field.kind === "hidden" && typeof field.default_value === "boolean")) parsed = value === "true";
        else if (field.kind === "json") {
          try { parsed = JSONValueSchema.parse(JSON.parse(value)); } catch { throw new Error(`${field.label}格式无效，请检查技术高级配置。`); }
          if (schema) { parsed = prepareConfiguration(parsed, schema); if (/\/pricing$/.test(operation.path) && operation.name !== "listing") validatePriceConfiguration(parsed); else if (/\/(catalog|routes)$/.test(operation.path)) validateModelConfiguration(parsed); }
        }
        else if (field.kind === "datetime") { const date = new Date(value); if (Number.isNaN(date.getTime())) throw new Error(`${field.label}时间无效`); parsed = date.toISOString(); }
        else { if (field.kind === "email" && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) throw new Error(`请填写有效的${field.label}`); parsed = value; }
        assign(payload, field.name, parsed);
      }
      if (operation.include_version && row?.version !== undefined) payload.version = row.version;
      if (Object.keys(activeFiles).length) { const data = new FormData(); for (const [key, value] of Object.entries(payload)) data.append(key, typeof value === "string" ? value : JSON.stringify(value)); for (const [key, file] of Object.entries(activeFiles)) data.append(key, file); await command.run(data); }
      else await command.run(payload);
    } catch (error) { const configPath = error instanceof ConfigurationError ? error.fieldPath : undefined; setValidationError(error instanceof Error ? error.message : "请检查输入"); setInvalidField(currentField); setInvalidConfigPath(configPath); focusField(currentField, configPath); }
  }
  const content = <><div className="mb-6 flex items-start justify-between gap-4"><div className="min-w-0"><p className="mb-2 text-xs font-medium tracking-wider text-brand-emphasis">业务办理</p><h2 className="text-xl font-semibold">{operation.label}</h2>{operation.description && <p className="mt-2 text-sm leading-7 text-ink-secondary">{operation.description}</p>}{objectName && <p className="mt-3 text-sm text-ink-secondary">办理对象：<span className="font-medium text-ink">{String(objectName)}</span></p>}</div>{onClose && <Button type="button" variant="ghost" size="sm" disabled={blocked} onClick={onClose}>{command.result ? "返回记录" : "关闭"}</Button>}</div>
    <ol aria-label="办理步骤" className="mb-7 grid grid-cols-3 gap-2 text-xs"><li className="flex items-center gap-2 text-brand-emphasis"><span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-brand-soft">{command.result ? <Check className="size-3" /> : "1"}</span>填写事项</li><li className="flex items-center gap-2 text-ink-secondary"><span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-canvas">2</span>核对影响</li><li className={`flex items-center gap-2 ${command.result ? "text-success" : "text-ink-mute"}`}><span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-canvas">{command.result ? <Check className="size-3" /> : "3"}</span>{command.result ? "已完成" : "提交办理"}</li></ol>
    <form ref={formRef} onSubmit={submit} noValidate className="space-y-7">
      {!command.result && groups.map(name => <section key={name} className="space-y-4">{!(fields.length === 1 && fields[0].kind === "json") && <div><h3 className="text-sm font-semibold">{name}</h3><p className="mt-1 text-xs leading-6 text-ink-mute">{groupHelp[name]}</p></div>}<div className="grid gap-5 sm:grid-cols-2">{fields.filter(field => group(field) === name).map(field => <div key={field.name} data-command-field={field.name} className={field.kind === "json" || field.kind === "textarea" || field.name === "grants" && field.options ? "min-w-0 sm:col-span-2" : "min-w-0"}>
        {field.kind === "json" && schema ? <ConfigurationEditor schema={schema} operatorID={operatorID} value={values[field.name] || ""} disabled={disabled || lockedFields.includes(field.name)} change={value => change(field.name, value)} /> : <Field field={field} primaryReference={primaryReference(operation, field)} value={values[field.name] || ""} disabled={disabled || lockedFields.includes(field.name)} invalid={invalidField === field.name} change={value => change(field.name, value)} onChoice={defaults => { if (disabled) return; setConfirmed(false); setValues(previous => { const next = { ...previous }; for (const [name, value] of Object.entries(defaults)) { const target = operation.fields.find(item => item.name === name); if (target && !secretField(target) && value !== undefined && value !== null && !lockedFields.includes(name)) next[name] = fieldString(target, value); } return next; }); }} onFile={file => { if (disabled) return; setConfirmed(false); setFiles(previous => ({ ...previous, [field.name]: file })); }} />}
        {lockedFields.includes(field.name) && <p className="mt-2 flex items-center gap-1.5 text-xs text-ink-mute"><LockKeyhole className="size-3" aria-hidden="true" />已按当前业务对象带入</p>}
        {invalidField === field.name && validationError && <p className="mt-2 text-xs text-danger">{validationError}</p>}
      </div>)}</div></section>)}
      {!command.result && <div className="rounded-control border border-hairline bg-canvas p-4"><h3 className="flex items-center gap-2 text-sm font-semibold"><ShieldCheck className="size-4 text-brand-emphasis" aria-hidden="true" />提交前核对</h3><Review fields={fields} values={values} files={files} /><p className="mt-3 text-xs leading-6 text-ink-mute">本次操作将保存在当前经营范围。服务器核实成功后，最新结果会同步到业务记录。</p>{operation.confirmation && <label className="mt-4 flex items-start gap-3 border-t border-hairline pt-4 text-sm leading-7"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} className="mt-2 size-4 shrink-0" disabled={disabled} />{operation.confirmation}</label>}</div>}
      {validationError && <div role="alert" className="rounded-control border border-danger/30 p-4 text-sm text-danger"><p>{validationError}</p>{invalidField && <button type="button" className="mt-2 underline underline-offset-4" onClick={() => focusField(invalidField, invalidConfigPath)}>定位需要修改的字段</button>}</div>}
      {!!command.error && <ErrorPanel error={command.error} />}
      {command.unresolved && <Notice><p className="font-medium text-ink">本次办理结果待核实</p><p>请保持此办理面板，查询业务记录或重试本次操作。重试会复用原提交内容与同一操作编号，避免重复办理。</p><details className="mt-2 text-xs"><summary className="cursor-pointer">查看核对编号</summary><span className="mt-1 block break-all font-mono">{command.actionKey}</span></details></Notice>}
      {command.result && <div role="status" className="rounded-control border border-success/30 bg-success/5 p-5"><CheckCircle2 className="mb-3 size-6 text-success" aria-hidden="true" /><h3 className="font-semibold">{operation.label}已受理</h3><p className="mt-2 text-sm leading-7 text-ink-secondary">{command.result.message || "本次办理已保存。返回业务记录即可查看最新进展及后续可办理事项。"}</p></div>}
      <div className="flex flex-wrap items-center gap-3 border-t border-hairline pt-5"><Button type="submit" disabled={command.busy || !scoped || !!command.result}>{command.busy ? "正在提交，请稍候…" : command.unresolved ? "重试本次操作" : command.result ? "本次操作已完成，关闭后查看最新记录" : operation.label}</Button>{!command.result && <p className="text-xs leading-6 text-ink-mute">{command.busy ? "正在保存，请保持页面。" : command.unresolved ? "原提交内容已锁定，等待核实。" : "请先核对填写内容与实际凭证。"}</p>}</div>
    </form>{command.result?.order && <PaymentAction order={command.result.order} />}</>;
  return embedded ? <section className="min-w-0">{content}</section> : <Card className="my-5">{content}</Card>;
}

function Review({ fields, values, files }: { fields: CommandField[]; values: Record<string, string>; files: Record<string, File> }) {
  const selected = fields.filter(field => values[field.name] || files[field.name]);
  if (!selected.length) return <p className="mt-3 text-sm text-ink-secondary">请核对当前记录及办理依据后确认提交。</p>;
  return <dl className="mt-3 grid gap-x-5 gap-y-3 sm:grid-cols-2">{selected.map(field => {
    if (field.kind === "json") return <div key={field.name} className="min-w-0 sm:col-span-2"><ConfigurationReview value={values[field.name]} /></div>;
    let content = field.kind === "file" ? files[field.name]?.name : values[field.name];
    if (secretField(field)) content = "已填写，提交时安全发送";
    else if (field.kind === "boolean") content = content === "true" ? "是" : "否";
    else if (field.kind === "select") content = field.options?.find(item => item.value === content)?.label || content;
    else if (field.name === "grants" && field.options) content = content?.split(",").filter(Boolean).map(value => field.options?.find(item => item.value === value)?.label || value).join("、") || "未追加自定义权限";
    return <div key={field.name} className="min-w-0"><dt className="text-xs text-ink-mute">{field.label.replace("配置 JSON", "业务配置")}</dt><dd className="mt-1 break-words text-sm text-ink-secondary">{content}</dd></div>;
  })}</dl>;
}

function ConfigurationReview({ value }: { value: string }) {
  let parsed: Record<string, JSONValue>;
  try { const next = JSON.parse(value); if (!next || typeof next !== "object" || Array.isArray(next)) throw new Error(); parsed = next; }
  catch { return <p className="text-sm text-danger">业务配置尚未完整，请检查上方字段。</p>; }
  const model = parsed.model as Record<string, JSONValue> | undefined, upstream = parsed.upstream as Record<string, JSONValue> | undefined, price = parsed.price as Record<string, JSONValue> | undefined;
  if (model || upstream) {
    const item = model || upstream!;
    return <div className="space-y-2 text-sm text-ink-secondary"><p className="font-medium text-ink">{model ? String(item.display_name || "待填写模型名称") : `上游模型：${String(item.model || "待填写")}`}</p><p>{model ? "公开模型" : "路由"}{item.enabled ? "将开放" : "保持停用"} · {Array.isArray(item.capabilities) ? item.capabilities.length : 0} 项已登记能力{upstream && ` · ${upstream.healthy ? "已核实健康" : "尚未确认健康"}`}</p><p className="text-xs">身份与版本：{String(item.identity || "待核实")}</p>{upstream && <p className="text-xs">选路优先级：{String(upstream.priority ?? 0)}。请核对路由与公开模型的身份、能力及计量是否一致。</p>}</div>;
  }
  if (price) {
    const scope = (price.scope || {}) as Record<string, JSONValue>, rule = (price.rule || {}) as Record<string, JSONValue>;
    const unit = scope.kind === "supplier" ? "USD" : scope.kind === "supply" ? "采购 ATG" : "ATG";
    const components = (Array.isArray(rule.components) ? rule.components : []) as Record<string, JSONValue>[];
    return <div className="space-y-3 text-sm text-ink-secondary"><p className="font-medium text-ink">{scope.kind === "supplier" ? "供应商成本价" : scope.kind === "supply" ? "对下级供货价" : "客户零售价"} · {price.layer === "promotion" ? "限时促销" : "常规价格"}</p>{rule.mode === "follow" ? <p>自动跟随上级价目，默认倍率 <strong className="break-all font-mono text-ink">{String(rule.multiplier || "待填写")}</strong>。{Object.keys((rule.model_multipliers || {}) as object).length} 项模型倍率覆盖。</p> : <ul className="space-y-2">{components.map((component, index) => <li key={index} className="rounded-control border border-hairline px-3 py-2"><span className="font-medium">{String(component.key || `组件 ${index + 1}`)}</span><span className="mt-1 block break-words">每 {String(component.per || "待填写")} {String(component.unit || "单位")}，基础单价 <span className="font-mono">{String(component.rate || "待填写")}</span> {unit}</span>{Array.isArray(component.tiers) && component.tiers.length > 0 && <span className="mt-1 block text-xs">另按已填写的 {component.tiers.length} 个连续阶梯计价，请核对每阶数量和单价。</span>}</li>)}</ul>}<p className="text-xs leading-6">价格按请求受理时固定。本次变更将按通知规则生效，并重算已售责任覆盖；已受理请求保留原价格。</p></div>;
  }
  if (parsed.action === "listing" || parsed.action === "authorization") return <div className="text-sm text-ink-secondary"><p className="font-medium text-ink">{parsed.action === "authorization" ? "调整直属下级模型授权" : "调整本经营方模型上架"}</p><p className="mt-2">本次将{parsed.enabled ? "允许" : "停止"}后续使用此模型；已受理请求按原责任处理。</p></div>;
  return <p className="text-sm text-ink-secondary">已填写业务配置，请核对上方条款与实际依据。</p>;
}

function Field({ field, value, disabled, invalid, change, onFile, onChoice, primaryReference }: { field: CommandField; value: string; disabled: boolean; invalid: boolean; change: (value: string) => void; onFile: (value: File) => void; onChoice: (values: Record<string, unknown>) => void; primaryReference: boolean }) {
  const id = useId();
  if (field.name === "grants" && field.options) return <PermissionField field={field} value={value} disabled={disabled} change={change} />;
  if (field.lookup_url) return <ReferenceField field={field} value={value} disabled={disabled} invalid={invalid} change={change} choose={onChoice} primary={primaryReference} />;
  const base = `mt-2 min-h-11 w-full rounded-control border ${invalid ? "border-danger" : "border-hairline"} bg-canvas-raised px-3 py-2 text-sm`;
  return <div className="text-sm"><label htmlFor={id}>{field.label}{field.required && <span className="ml-1 text-danger">*</span>}</label>
    {field.kind === "boolean" ? <label className="mt-2 flex min-h-11 items-center gap-3 rounded-control border border-hairline px-3"><input id={id} type="checkbox" checked={value === "true"} disabled={disabled} onChange={event => change(event.target.checked ? "true" : "false")} />{value === "true" ? "已启用" : "未启用"}</label> : field.kind === "select" ? <select id={id} aria-invalid={invalid || undefined} className={base} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)}><option value="">请选择</option>{field.options?.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select> : ["textarea", "json"].includes(field.kind) ? <textarea id={id} aria-invalid={invalid || undefined} className={`${base} min-h-28 ${field.kind === "json" ? "font-mono" : ""}`} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)} spellCheck={field.kind !== "json"} /> : field.kind === "file" ? <Input id={id} aria-invalid={invalid || undefined} className="mt-2" type="file" required={field.required} disabled={disabled} onChange={event => { const file = event.target.files?.[0]; if (file) onFile(file); }} /> : <Input id={id} aria-invalid={invalid || undefined} className="mt-2" type={field.kind === "email" ? "email" : field.kind === "password" ? "password" : field.kind === "datetime" ? "datetime-local" : "text"} inputMode={field.kind === "decimal" ? "decimal" : field.kind === "integer" ? "numeric" : undefined} autoComplete={field.kind === "password" ? "new-password" : "off"} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)} />}
    {field.help && <p className="mt-2 text-xs leading-6 text-ink-mute">{field.help}</p>}
  </div>;
}

function PermissionField({ field, value, disabled, change }: { field: CommandField; value: string; disabled: boolean; change: (value: string) => void }) {
  const selected = value.split(",").map(item => item.trim()).filter(Boolean);
  const options = [...(field.options || []), ...selected.filter(item => !field.options?.some(option => option.value === item)).map(item => ({ value: item, label: `现有扩展权限 · ${item}` }))];
  return <fieldset className="text-sm"><legend>{field.label}</legend><div className="mt-2 grid gap-2 sm:grid-cols-2">{options.map(option => <label key={option.value} className="flex min-h-11 items-center gap-2 rounded-control border border-hairline px-3 py-2"><input type="checkbox" aria-label={option.label} checked={selected.includes(option.value)} disabled={disabled} onChange={event => change((event.target.checked ? [...selected, option.value] : selected.filter(item => item !== option.value)).join(","))} />{option.label}</label>)}</div>{field.help && <p className="mt-2 text-xs leading-6 text-ink-mute">{field.help}</p>}</fieldset>;
}

const ReferenceSchema = z.object({ items: z.array(z.object({ value: z.string(), label: z.string(), defaults: z.record(z.string(), z.unknown()).optional() })), next_cursor: z.string().optional() });
function ReferenceField({ field, value, disabled, invalid, change, choose, primary }: { field: CommandField; value: string; disabled: boolean; invalid: boolean; change: (value: string) => void; choose: (values: Record<string, unknown>) => void; primary: boolean }) {
  const [search, setSearch] = useState(""); const deferred = useDeferredValue(search); const [cursor, setCursor] = useState(""); const [selectedLabel, setSelectedLabel] = useState(""); const path = field.lookup_url!;
  const data = useQuery({ queryKey: ["references", path, deferred, cursor], queryFn: ({ signal }) => request(`${path}?q=${encodeURIComponent(deferred)}&cursor=${encodeURIComponent(cursor)}`, ReferenceSchema, { signal }) });
  function select(next: string) {
    change(next);
    const option = data.data?.items.find(item => item.value === next);
    setSelectedLabel(option?.label || "");
    const defaults = option?.defaults || {};
    if (primary) { choose({ expected_version: 0, ...defaults }); return; }
    // Linked records have their own identities and revisions. Carry only the
    // dependent relationship fields, never another record's CAS or enabled flag.
    const permitted = new Set([field.name]);
    if (field.name === "quote_id" && /\/purchase-quotes$/.test(path)) permitted.add("supplier_id");
    if (field.name === "order_id" && /\/orders$/.test(path)) permitted.add("customer_id");
    if (fieldName(field) === "receipt_id" && /\/receipts$/.test(path)) ["amount_usd", "currency", "issuer_id"].forEach(key => permitted.add(key));
    if (field.name === "scope_id" && /\/scope-subjects$/.test(path)) permitted.add("scope_kind");
    if (field.name === "asset_network_version_id" && /\/payment-assets$/.test(path)) permitted.add("driver");
    choose(Object.fromEntries(Object.entries(defaults).filter(([key]) => permitted.has(key) && !["id", "version", "expected_version"].includes(key))));
  }
  return <fieldset className="min-w-0 text-sm"><legend>{field.label}{field.required && <span className="ml-1 text-danger">*</span>}</legend><Input className="mt-2" aria-label={`搜索${field.label}`} placeholder="按名称或编号搜索" value={search} disabled={disabled} onChange={event => { setSearch(event.target.value); setCursor(""); }} /><select aria-label={field.label} aria-invalid={invalid || undefined} className={`mt-2 min-h-11 w-full min-w-0 rounded-control border ${invalid ? "border-danger" : "border-hairline"} bg-canvas-raised px-3 py-2 text-sm`} value={value} required={field.required} disabled={disabled || data.isPending} onChange={event => select(event.target.value)}><option value="">{field.required ? "请选择实际业务记录" : "新建 / 不选择现有记录"}</option>{value && !data.data?.items.some(item => item.value === value) && <option value={value}>{selectedLabel || value}</option>}{data.data?.items.map(item => <option value={item.value} key={item.value}>{item.label}</option>)}</select>{data.isError && <div className="mt-2"><ErrorPanel error={data.error} retry={() => void data.refetch()} compact /></div>}{data.data?.next_cursor && <Button variant="outline" size="sm" type="button" className="mt-2" disabled={disabled} onClick={() => setCursor(data.data!.next_cursor!)}>更多记录</Button>}{field.help && <p className="mt-2 text-xs leading-6 text-ink-mute">{field.help}</p>}</fieldset>;
}
