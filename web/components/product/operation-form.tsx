"use client";
import { useI18n } from "@/components/i18n";
import { useProduct } from "./context";
import { FieldHelp } from "./field-help";
import { businessFieldHelp } from "./business-field-help";
import { YesNoChoice } from "./yes-no";

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
import { closePaymentWindow, reservePaymentWindow, submitPaymentForm } from "@/lib/payment-window";
import { ErrorPanel, Notice, useCommand } from "./shared";
import { configDefaults, configurationSchema, ConfigurationError, prepareConfiguration, presentSupplierConfiguration, retainKnownFields, validateModelConfiguration, validatePriceConfiguration } from "./operation-config";
import { RecordPicker, type RecordChoice } from "./record-picker";
import { PurchaseTiersEditor, PurchaseTierSummary, PurchasePreview, purchaseTiers, calculatePurchase } from "./purchase-tiers";
import { ConfigurationEditor } from "./operation-config-editor";
import { PriceRates } from "./pricing-cards";

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
      if (row && /\/routes$/.test(operation.path)) source = { route: row.values, expected_version: row.version ?? 0 };
      if (row && /\/providers$/.test(operation.path) && operation.name === "model") source = presentSupplierConfiguration({ supplier_model: row.values, expected_version: row.version ?? 0 });
      const defaults = configDefaults(schema) || {};
      const merged = source === undefined ? defaults : mergeValues(defaults, JSONValueSchema.parse(source));
      return JSON.stringify(retainKnownFields(merged, schema));
    }
  }
  if ((fieldName(field) === "expected_version" || fieldName(field) === "version") && row?.version !== undefined) return String(row.version);
  return fieldString(field, nested(row?.values, field.name) ?? field.default_value);
}
function matchesVisibility(field: CommandField, values: Record<string, string>, row?: Entity) {
  if (!field.visible_when) return true;
  const condition = field.visible_when;
  const value = [condition.field, ...(condition.fallback_fields || [])].map(name => values[name] || nested(row?.values, name)).find(value => value !== undefined && value !== null && String(value).trim() !== "");
  const matches = condition.values.includes(String(value || "").trim().toUpperCase());
  // An unknown currency must keep real FX evidence visible and required.
  return condition.not ? !matches : condition.values.includes(String(value || ""));
}
function visible(field: CommandField, values: Record<string, string>, row?: Entity) { return field.kind !== "hidden" && matchesVisibility(field, values, row); }
function group(field: CommandField): string {
  if (field.name.startsWith("credentials.") || field.name === "api_base_url") return "通道接入材料";
  if (/evidence|reference|reason|explanation|terms/.test(field.name) && !field.lookup_url) return "依据与办理说明";
  if (/(_at$|^from$|^to$|timezone|period|seconds|days)/.test(field.name)) return "时间与有效期";
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
function groupIntro(name: string, groupFields: CommandField[]) {
  if (name === "办理信息" && groupFields.every(field => !field.lookup_url)) return "填写本次要保存的内容。";
  if(name==="金额、额度与办理规则" && !groupFields.some(field=>["direct_rate","indirect_rate","concession_rate","fee_rate","maximum_ratio"].includes(fieldName(field)))) return "按字段标示的 USD、ATG 或原币种填写，精确值按原样提交。";
  return groupHelp[name];
}

type OperationFormProps = {
  operation: Operation;
  row?: Entity;
  operatorID?: string;
  initialValues?: Record<string, JSONValue>;
  lockedFields?: string[];
  embedded?: boolean;
  onBlockedChange?: (blocked: boolean) => void;
  onClose?: () => void;
  onAbandon?: () => void;
};

export function OperationForm({ operation, row, operatorID, initialValues, lockedFields = [], embedded = false, onBlockedChange, onClose, onAbandon }: OperationFormProps) {
  const { t, locale } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const path = operation.path.replaceAll("{id}", encodeURIComponent(row?.id || "")).replaceAll("{operator_id}", encodeURIComponent(operatorID || ""));
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(operation.fields.map(field => [field.name, initial(field, operation, row, initialValues, operatorID)])));
  const [files, setFiles] = useState<Record<string, File>>({});
  const [referenceLabels, setReferenceLabels] = useState<Record<string, string>>(() => row?.presentation?.labels || {});
  const [confirmed, setConfirmed] = useState(false);
  const [configurationPending, setConfigurationPending] = useState(false);
  const [validationError, setValidationError] = useState<string | ConfigurationError>();
  const [invalidField, setInvalidField] = useState<string>();
  const [invalidConfigPath, setInvalidConfigPath] = useState<string>();
  const formRef = useRef<HTMLFormElement>(null);
  const command = useCommand(path, CommandResultSchema, operation.method);
  // Money movements stay on the panel until the same attempt is verified.
  // Model setup can be abandoned when the result is unknown.
  const canLeave = /\/(catalog|providers|routes)$/.test(operation.path);
  const frozen = command.busy || command.unresolved || !!command.result;
  const dismissLocked = command.busy || (command.unresolved && !canLeave);
  const disabled = frozen;
  const scoped = operatorID ? path.startsWith(`/api/v1/operators/${encodeURIComponent(operatorID)}/`) : path.startsWith("/api/v1/me/");
  const fields = operation.fields.filter(field => visible(field, values, row));
  const simpleProduct = /\/(pricing|plans|catalog|providers|routes|bifrost|mail|operators|procurement|coverage|contracts|branding|channel-themes|domains)$/.test(operation.path);
  const fieldGroup = (field: CommandField) => (/\/operators$/.test(operation.path) && ["supply_terms","contract_attachment_id"].includes(field.name) || /\/procurement$/.test(operation.path) && operation.name==="quote" && ["name","terms_summary","contract_attachment_id","valid_until"].includes(field.name)) ? "补充约定（可选）" : /\/plans$/.test(operation.path) ? (["new_purchase_enabled","renewal_enabled"].includes(field.name) ? "销售与续费" : field.name==="terms_summary" ? "补充说明" : "套餐内容") : group(field);
  const groups = Array.from(new Set(fields.map(fieldGroup)));
  const schema = configurationSchema(operation, operatorID);
  const objectName = row?.presentation?.title || row && ["display_name", "name", "plan_name", "customer_name", "title"].map(key => row.values[key]).find(value => typeof value === "string" && value);

  useEffect(() => { onBlockedChange?.(dismissLocked); }, [dismissLocked, onBlockedChange]);
  useEffect(() => {
    if (!command.busy && !command.unresolved) return;
    const guard = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ""; };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [command.busy, command.unresolved]);
  function leaveUnresolved() { command.reset(); (onAbandon || onClose)?.(); }
  function focusField(name?: string, configPath?: string) {
    const configTarget = configPath && Array.from(formRef.current?.querySelectorAll<HTMLElement>("[data-config-path]") || []).find(element => element.dataset.configPath === configPath);
    const target = Array.from(formRef.current?.querySelectorAll<HTMLElement>("[data-command-field]") || []).find(element => element.dataset.commandField === name);
    const destination = configTarget || target;
    (destination?.querySelector<HTMLElement>("input:not([type=hidden]):not(:disabled),select:not(:disabled),textarea:not(:disabled)") || destination?.querySelector<HTMLElement>("button:not(:disabled):not([aria-haspopup=dialog])"))?.focus();
    destination?.scrollIntoView?.({ behavior: "smooth", block: "center" });
  }
  function change(name: string, value: string) {
    if (disabled || lockedFields.includes(name)) return;
    setValues(previous => ({ ...previous, [name]: value, ...(/\/exports$/.test(operation.path) && name === "kind" ? {owner_id:""} : {}), ...(/\/supplier-costs$/.test(operation.path) && name === "provider_id" ? {supplier_model_id:""} : {}) }));
    setConfirmed(false);
    if (invalidField === name) { setInvalidField(undefined); setValidationError(undefined); }
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setValidationError(undefined); setInvalidField(undefined); setInvalidConfigPath(undefined);
    if (!scoped) { setValidationError("操作不属于当前经营空间，已阻止提交。"); return; }
    if (command.busy || command.result || configurationPending) return;
    let currentField: string | undefined;
    try {
      if (operation.confirmation && !confirmed) throw new Error("请阅读并确认本次办理影响后提交。");
      const payload: Record<string, JSONValue> = {};
      const activeFiles: Record<string, File> = {};
      for (const field of operation.fields) {
        if (!matchesVisibility(field, values, row)) continue;
        currentField = field.name;
        const value = values[field.name] || "";
        if (field.kind === "file") { if (field.required && !files[field.name]) throw new ConfigurationError("请上传{label}", "", { label: field.label }); if (files[field.name]) activeFiles[field.name] = files[field.name]; continue; }
        if (!value && !field.required) { if (field.kind === "csv") assign(payload, field.name, []); else if (field.name.startsWith("values.") || fieldName(field) === "second_review_above") assign(payload, field.name, null); continue; }
        if (!value && field.required) throw new ConfigurationError("请填写{label}", "", { label: field.label });
        if (field.name === "initial_password" && value && (value.length < 10 || new TextEncoder().encode(value).length > 72)) throw new ConfigurationError("新账号初始密码至少 10 位，最长 72 字节。", "");
        let parsed: JSONValue;
        if (field.kind === "csv") parsed = value.split(",").map(item => item.trim()).filter(Boolean);
        else if (field.kind === "decimal") {
          const signed = /\/(risk|alerts)$/.test(operation.path) && ["warning", "critical"].includes(fieldName(field));
          if (!(signed ? SignedDecimal : Decimal).safeParse(value).success || (value.split(".")[1]?.length || 0) > 18) throw new ConfigurationError(signed ? "{label}请输入十进制数，最多 18 位小数" : "{label}请输入非负十进制数，最多 18 位小数", "", { label: field.label });
          if (["direct_rate", "indirect_rate", "concession_rate", "fee_rate", "maximum_ratio"].includes(fieldName(field)) && compareDecimal(value, "1") > 0) throw new ConfigurationError("{label}应使用 0 到 1 的精确小数，例如 5% 填写 0.05", "", { label: field.label });
          parsed = value;
        }
        else if (field.kind === "integer" || fieldName(field) === "expected_version" || fieldName(field) === "version") { if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) throw new ConfigurationError("{label}应为非负整数", "", { label: field.label }); parsed = Number(value); }
        else if (field.kind === "boolean" || (field.kind === "hidden" && typeof field.default_value === "boolean")) parsed = value === "true";
        else if (field.kind === "json") {
          try { parsed = JSONValueSchema.parse(JSON.parse(value)); } catch { throw new ConfigurationError("{label}格式无效，请检查上方填写的内容。", "", { label: field.label }); }
          if (field.name === "gift_tiers") purchaseTiers(parsed);
          if (schema) { parsed = prepareConfiguration(parsed, schema); if (/\/pricing$/.test(operation.path) && !["listing", "template"].includes(operation.name)) validatePriceConfiguration(parsed); else if (operation.name === "template" || /\/(catalog|routes)$/.test(operation.path) || (/\/providers$/.test(operation.path) && operation.name === "model")) validateModelConfiguration(parsed); }
        }
        else if (field.kind === "datetime") { const date = new Date(value); if (Number.isNaN(date.getTime())) throw new ConfigurationError("{label}时间无效", "", { label: field.label }); parsed = date.toISOString(); }
        else { if (field.kind === "email" && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) throw new ConfigurationError("请填写有效的{label}", "", { label: field.label }); parsed = value; }
        assign(payload, field.name, parsed);
      }
      if (operation.name === "purchase" && /\/procurement\//.test(operation.path)) calculatePurchase(row?.values.gift_tiers,values.amount_usd);
      if (operation.include_version && row?.version !== undefined) payload.version = row.version;
      const kvoucher = operation.fields.some(field => {
        if (field.name !== "merchant_id" && field.name !== "external_method_id") return false;
        const selected = values[field.name] || "";
        const label = field.options?.find(item => item.value === selected)?.label || referenceLabels[field.name] || "";
        return label.toLowerCase().includes("kvoucher");
      });
      const payWindow = kvoucher ? reservePaymentWindow() : null;
      const result = Object.keys(activeFiles).length ? await command.run((() => { const data = new FormData(); for (const [key, value] of Object.entries(payload)) data.append(key, typeof value === "string" ? value : JSON.stringify(value)); for (const [key, file] of Object.entries(activeFiles)) data.append(key, file); return data; })()) : await command.run(payload);
      const payment = result?.order?.payment_action;
      if (payment?.kind === "form" && payment.url && payment.fields) submitPaymentForm(payWindow, payment.url, payment.fields);
      else closePaymentWindow(payWindow);
    } catch (error) { const configPath = error instanceof ConfigurationError ? error.fieldPath : undefined; setValidationError(error instanceof ConfigurationError ? error : error instanceof Error ? error.message : "请检查输入"); setInvalidField(currentField); setInvalidConfigPath(configPath); focusField(currentField, configPath); }
  }
  const content = <><div className="mb-6 flex items-start justify-between gap-4"><div className="min-w-0">{!embedded && <><p className="th-eyebrow mb-2 text-brand-emphasis">{t("业务办理")}</p><h2 className="text-xl font-semibold">{t(operation.label)}</h2></>}{operation.description && <p className="mt-2 text-sm leading-7 text-ink-secondary">{t(operation.description)}</p>}{objectName && !embedded && <p className="mt-3 text-sm text-ink-secondary">{t("办理对象：")}<span className="font-medium text-ink">{String(objectName)}</span></p>}</div>{onClose && <Button type="button" variant="ghost" size="sm" disabled={dismissLocked} onClick={onClose}>{command.result ? t("返回记录") : t("关闭")}</Button>}</div>
    {!simpleProduct && <ol aria-label={t("办理步骤")} className="th-stepper"><li className="flex items-center gap-2 text-brand-emphasis"><span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-brand-soft">{command.result ? <Check className="size-3" /> : "1"}</span>{t("填写事项")}</li><li className="flex items-center gap-2 text-ink-secondary"><span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-canvas">2</span>{t("核对影响")}</li><li className={`flex items-center gap-2 ${command.result ? "text-success" : "text-ink-mute"}`}><span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-canvas">{command.result ? <Check className="size-3" /> : "3"}</span>{command.result ? t("已完成") : t("提交办理")}</li></ol>}
    <form ref={formRef} onSubmit={submit} noValidate className="space-y-7">
      {!command.result && /\/exits\/[^/]+\/verify_manual$/.test(operation.path) && !!nested(row?.values,"proof.recipient_reference") && <Notice><p>{t("已核实的原付款人账户：{account}", {account:String(nested(row?.values,"proof.recipient_reference"))})}</p></Notice>}
      {!command.result && groups.map(name => { const Container = name==="补充约定（可选）" ? "details" : "section"; return <Container key={name} className="space-y-4">{name==="补充约定（可选）" && <summary className="cursor-pointer text-sm font-medium">{t(name)}</summary>}{name!=="补充约定（可选）" && !(fields.length === 1 && fields[0].kind === "json") && <div><h3 className="text-sm font-semibold">{t(name)}</h3>{!simpleProduct && <p className="mt-1 text-xs leading-6 text-ink-mute">{t(groupIntro(name, fields.filter(field => fieldGroup(field) === name)))}</p>}</div>}<div className="grid gap-5 sm:grid-cols-2">{fields.filter(field => fieldGroup(field) === name).map(field => <div key={field.name} data-command-field={field.name} className={field.kind === "json" || field.kind === "textarea" || (field.kind === "csv" || field.name === "grants") && field.options ? "min-w-0 sm:col-span-2" : "min-w-0"}>
        {field.name === "gift_tiers" ? <PurchaseTiersEditor value={values[field.name] || ""} disabled={disabled} change={value => change(field.name,value)} /> : field.kind === "json" && schema ? <ConfigurationEditor onPendingChange={setConfigurationPending} schema={schema} allowFollow={workspace?.operator.kind !== "platform"} allowDownstream={workspace?.operator.kind !== "channel"} modelProfiles={row?.values.billing} operatorID={operatorID} value={values[field.name] || ""} disabled={disabled || configurationPending || lockedFields.includes(field.name)} change={value => change(field.name, value)} /> : <Field field={{ ...field, lookup_url: field.name === "owner_id" && field.lookup_url && /\/exports$/.test(operation.path) ? `${field.lookup_url}?kind=${encodeURIComponent(values.kind || "")}` : field.name === "supplier_model_id" && field.lookup_url && /\/supplier-costs$/.test(operation.path) ? `${field.lookup_url}?provider_id=${encodeURIComponent(values.provider_id || "")}` : field.lookup_url, help: businessFieldHelp(field.name === "warning" && /\/coverage(?:[?]|$)/.test(operation.path) ? "coverage.warning" : field.name, field.help, field.lookup_url) }} onLabel={label => setReferenceLabels(previous => previous[field.name] === label ? previous : { ...previous, [field.name]: label })} primaryReference={primaryReference(operation, field)} value={values[field.name] || ""} disabled={disabled || lockedFields.includes(field.name)} invalid={invalidField === field.name} change={value => change(field.name, value)} onChoice={defaults => { if (disabled) return; setConfirmed(false); setValues(previous => { const next = { ...previous }; for (const [name, value] of Object.entries(defaults)) { const target = operation.fields.find(item => item.name === name); if (target && !secretField(target) && value !== undefined && value !== null && !lockedFields.includes(name)) next[name] = fieldString(target, value); } return next; }); }} onFile={file => { if (disabled) return; setConfirmed(false); setFiles(previous => ({ ...previous, [field.name]: file })); }} />}
        {lockedFields.includes(field.name) && <p className="mt-2 flex items-center gap-1.5 text-xs text-ink-mute"><LockKeyhole className="size-3" aria-hidden="true" />{t("已按当前业务对象带入")}</p>}
        {invalidField === field.name && validationError && <p className="mt-2 text-xs text-danger">{validationError instanceof ConfigurationError ? validationError.localized(locale) : t(validationError)}</p>}
      </div>)}</div></Container>; })}
      {!command.result && operation.name === "purchase" && /\/procurement\//.test(operation.path) && <PurchasePreview tiers={row?.values.gift_tiers} amount={values.amount_usd || ""} />}
      {!command.result && (!simpleProduct || operation.confirmation) && <div className="th-review"><h3 className="flex items-center gap-2 text-sm font-semibold"><ShieldCheck className="size-4 text-brand-emphasis" aria-hidden="true" />{t("提交前核对")}</h3><Review fields={fields} values={values} files={files} referenceLabels={referenceLabels} /><p className="mt-3 text-xs leading-6 text-ink-mute">{t("本次操作将保存在当前经营范围。服务器核实成功后，最新结果会同步到业务记录。")}</p>{operation.confirmation && <label className="mt-4 flex items-start gap-3 border-t border-hairline pt-4 text-sm leading-7"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} className="mt-2 size-4 shrink-0" disabled={disabled} />{t(operation.confirmation)}</label>}</div>}
      {validationError && <div role="alert" className="rounded-control border border-danger/30 p-4 text-sm text-danger"><p>{validationError instanceof ConfigurationError ? validationError.localized(locale) : t(validationError)}</p>{invalidField && <button type="button" className="mt-2 underline underline-offset-4" onClick={() => focusField(invalidField, invalidConfigPath)}>{t("定位需要修改的字段")}</button>}</div>}
      {!!command.error && <ErrorPanel error={command.error} />}
      {command.unresolved && <Notice><p className="font-medium text-ink">{t("本次办理结果待核实")}</p>{canLeave ? <p>{t("可以重试原来的内容。写错了或改变主意时，放弃并关闭，回到列表后重新填写。如果列表里已经有这条记录，不要用同一个标识再添加一次。")}</p> : <p>{t("请保持此办理面板，查询业务记录或重试本次操作。重试会复用原提交内容与同一操作编号，避免重复办理。")}</p>}<div className="mt-3 flex flex-wrap items-center gap-3">{canLeave && <Button type="button" variant="outline" size="sm" onClick={leaveUnresolved}>{t("放弃并关闭")}</Button>}<details className="text-xs"><summary className="cursor-pointer">{t("查看核对编号")}</summary><span className="mt-1 block break-all font-mono">{command.actionKey}</span></details></div></Notice>}
      {command.result && <div role="status" className="rounded-control border border-success/30 bg-success/5 p-5"><CheckCircle2 className="mb-3 size-6 text-success" aria-hidden="true" /><h3 className="font-semibold">{t("{operation}已受理", { operation: t(operation.label) })}</h3><p className="mt-2 text-sm leading-7 text-ink-secondary">{t(command.result.message || "本次办理已保存。返回业务记录即可查看最新进展及后续可办理事项。")}</p>{command.result.access_url && <AccessHandoff url={command.result.access_url} email={command.result.email} password={values.initial_password} requiresPasswordChange={command.result.requires_password_change} />}</div>}
      <div className="th-form-actions"><Button type="submit" disabled={command.busy || configurationPending || !scoped || !!command.result}>{command.busy ? t("正在提交，请稍候…") : command.unresolved ? t("重试本次操作") : command.result ? t("本次操作已完成，关闭后查看最新记录") : t(operation.label)}</Button>{!command.result && <p className="text-xs leading-6 text-ink-mute">{command.busy ? t("正在保存，请保持页面。") : command.unresolved ? (canLeave ? t("可以放弃并关闭，回到列表后重新填写。") : t("原提交内容已锁定，等待核实。")) : t(/\/(pricing|plans)$/.test(operation.path) ? "保存后系统会自动处理版本与通知，已购权益不重算。" : simpleProduct ? "保存后配置生效。" : "请先核对填写内容与实际凭证。")}</p>}</div>
    </form>{command.result?.order && <PaymentAction order={command.result.order} onChangeMethod={operation.name === "checkout" && !/\/procurement\//.test(operation.path) ? () => command.reset() : undefined} />}</>;
  return embedded ? <section className="min-w-0">{content}</section> : <Card className="my-5">{content}</Card>;
}

function AccessHandoff({ url, email, password, requiresPasswordChange }: { url: string; email?: string; password?: string; requiresPasswordChange?: boolean }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const safe = url.startsWith("/") && !url.startsWith("//") || /^https?:\/\//.test(url);
  if (!safe) return null;
  return <div className="mt-4 space-y-3 border-t border-success/20 pt-4 text-sm">{email && <p>{t("登录邮箱")}：{email}</p>}<a className="break-all text-brand-emphasis underline" href={url} target="_blank" rel="noreferrer">{t(requiresPasswordChange ? "打开登录入口" : "打开接受邀请链接")}</a>{requiresPasswordChange && password && <p>{t("初始密码仅在本次办理中显示，请安全交给本人；首次登录须修改密码。")}</p>}<div><Button type="button" variant="outline" size="sm" onClick={async () => { try { const target = new URL(url, window.location.origin).href; await navigator.clipboard.writeText([email, requiresPasswordChange ? password : "", target].filter(Boolean).join("\n")); setCopied(true); } catch { setCopied(false); } }}>{t(copied ? "已复制" : "复制开通信息")}</Button></div></div>;
}

function Review({ fields, values, files, referenceLabels }: { fields: CommandField[]; values: Record<string, string>; files: Record<string, File>; referenceLabels: Record<string, string> }) {
  const { t } = useI18n();
  const selected = fields.filter(field => values[field.name] || files[field.name]);
  if (!selected.length) return <p className="mt-3 text-sm text-ink-secondary">{t("请核对当前记录及办理依据后确认提交。")}</p>;
  return <dl className="mt-3 grid gap-x-5 gap-y-3 sm:grid-cols-2">{selected.map(field => {
    if (field.name === "gift_tiers") { let tiers:unknown;try{tiers=JSON.parse(values[field.name]);}catch{}return <div key={field.name} className="sm:col-span-2"><PurchaseTierSummary value={tiers} /></div>; }
    if (field.kind === "json") return <div key={field.name} className="min-w-0 sm:col-span-2"><ConfigurationReview value={values[field.name]} /></div>;
    let content = field.kind === "file" ? files[field.name]?.name : values[field.name];
    if (secretField(field)) content = t("已填写，提交时安全发送");
    else if (field.kind === "boolean") content = content === "true" ? t("是") : t("否");
    else if (field.lookup_url) content = referenceLabels[field.name] || t("已选择关联记录");
    else if (field.kind === "select") content = field.options?.find(item => item.value === content)?.label || content;
    else if ((field.kind === "csv" || field.name === "grants") && field.options) content = content?.split(",").filter(Boolean).map(value => t(field.options?.find(item => item.value === value)?.label || (field.name === "grants" ? value : t("已选择的现有人员")))).join(t("、")) || t("未追加自定义权限");
    return <div key={field.name} className="min-w-0"><dt className="text-xs text-ink-mute">{t(field.label.replace("配置 JSON", "业务配置"))}</dt><dd className="mt-1 break-words text-sm text-ink-secondary">{field.kind === "select" || field.kind === "csv" && field.options || field.name === "grants" ? t(content || "") : content}</dd></div>;
  })}</dl>;
}

function ConfigurationReview({ value }: { value: string }) {
  const { t } = useI18n();
  let parsed: Record<string, JSONValue>;
  try { const next = JSON.parse(value); if (!next || typeof next !== "object" || Array.isArray(next)) throw new Error(); parsed = next; }
  catch { return <p className="text-sm text-danger">{t("业务配置尚未完整，请检查上方字段。")}</p>; }
  const model = parsed.model as Record<string, JSONValue> | undefined, route = parsed.route as Record<string, JSONValue> | undefined, supplier = parsed.supplier_model as Record<string, JSONValue> | undefined, price = parsed.price as Record<string, JSONValue> | undefined;
  if (model) {
    return <div className="space-y-2 text-sm text-ink-secondary"><p className="font-medium text-ink">{String(model.id || t("待填写公开模型标识"))}</p><p>{Array.isArray(model.capabilities) ? model.capabilities.length : 0} {t("项已登记能力")}</p><p>{t("是否对客户开放，在公开模型列表里切换。没有配置路由时不能打开。")}</p></div>;
  }
  if (route) {
    const strategy = ({ priority: "固定优先级", lowest_latency: "已验证最低延迟", lowest_cost: "已验证最低成本" } as Record<string, string>)[String(route.strategy || "")] || String(route.strategy || "");
    return <p className="text-sm text-ink-secondary">{t("公开模型 {model} 绑定了 {count} 个供应商模型，选路策略是{strategy}。", { model: String(route.public_model_id || t("待选择")), count: Array.isArray(route.bindings) ? route.bindings.length : 0, strategy: t(strategy || "待选择") })}</p>;
  }
  if (supplier) {
    return <p className="text-sm text-ink-secondary">{t("供应商模型 {name} 登记了 {count} 项能力，成本使用供应商报告或 Bifrost 参考价目核算。", { name: String(supplier.model || t("待填写")), count: Array.isArray(supplier.capabilities) ? supplier.capabilities.length : 0 })}</p>;
  }
  if (parsed.action === "billing") {
    const billing = parsed.billing as Record<string, JSONValue> || {};
    const modality:Record<string,string>={all:"全部模态（统一 Token）",text:"文本",image:"图片",audio:"音频",video:"视频"};
    const method:Record<string,string>={tokens:"按 Token",characters:"按字符",images:"按张",megapixels:"按百万像素",seconds:"按秒"};
    return <div className="space-y-2 text-sm text-ink-secondary">{["input","output"].map(direction=><p key={direction}>{t(direction==="input"?"输入":"输出")}：{Array.isArray(billing[direction]) && billing[direction].length ? (billing[direction] as Record<string,JSONValue>[]).map(item=>`${t(modality[String(item.modality)] || String(item.modality))} · ${t(method[String(item.template)] || String(item.template))}`).join(t("、")) : t("不计费")}</p>)}<p className="text-xs">{t("系统自动安排变更通知和生效，不影响已受理请求。")}</p></div>;
  }
  if (price) {
    const scope = (price.scope || {}) as Record<string, JSONValue>, rule = (price.rule || {}) as Record<string, JSONValue>;
    const unit = scope.kind === "supplier" ? "USD" : scope.kind === "supply" ? t("采购 ATG") : "ATG";
    const components = (Array.isArray(rule.components) ? rule.components : []) as Record<string, JSONValue>[];
    return <div className="space-y-3 text-sm text-ink-secondary"><p className="font-medium text-ink">{scope.kind === "supplier" ? t("供应商成本价") : scope.kind === "supply" ? t("对下级供货价") : t("客户零售价")} · {price.layer === "promotion" ? t("限时促销") : t("常规价格")}</p>{rule.mode === "follow" ? <p>{t("跟随上级供货价，倍率")} <strong className="break-all font-mono text-ink">{String(rule.multiplier || t("待填写"))}</strong></p> : <PriceRates components={components} />}<p className="text-xs leading-6">{t("价格按请求受理时固定。本次变更将按通知规则生效，并重算已售责任覆盖；已受理请求保留原价格。")}</p></div>;
  }
  if (parsed.action === "listing" || parsed.action === "authorization") return <div className="text-sm text-ink-secondary"><p className="font-medium text-ink">{parsed.action === "authorization" ? t("调整直属下级模型授权") : t("调整本经营方模型上架")}</p><p className="mt-2">{t(parsed.enabled ? "本次将允许后续使用此模型；已受理请求按原责任处理。" : "本次将停止后续使用此模型；已受理请求按原责任处理。")}</p></div>;
  return <p className="text-sm text-ink-secondary">{t("已填写业务配置，请核对上方条款与实际依据。")}</p>;
}

function Field({ field, value, disabled, invalid, change, onFile, onChoice, onLabel, primaryReference }: { field: CommandField; value: string; disabled: boolean; invalid: boolean; change: (value: string) => void; onFile: (value: File) => void; onChoice: (values: Record<string, unknown>) => void; onLabel: (label: string) => void; primaryReference: boolean }) {
  const { t } = useI18n();
  const id = useId();
  if ((field.kind === "csv" || field.name === "grants") && field.options) return <PermissionField field={field} value={value} disabled={disabled} change={change} />;
  if (field.lookup_url) return <ReferenceField field={field} value={value} disabled={disabled} invalid={invalid} change={change} choose={onChoice} onLabel={onLabel} primary={primaryReference} />;
  if (field.kind === "boolean") return <div className="text-sm"><span id={id}>{t(field.label)}</span>{field.required && <span className="ml-1 text-danger" aria-hidden="true">*</span>}{field.help && <FieldHelp title={t(field.label)}>{t(field.help)}</FieldHelp>}<YesNoChoice labelledBy={id} name={field.name} value={value === "true"} disabled={disabled} onChange={next => change(next ? "true" : "false")} /></div>;
  const base = `mt-2 min-h-11 w-full rounded-control border ${invalid ? "border-danger" : "border-hairline"} bg-canvas-raised px-3 py-2 text-sm`;
  return <div className="text-sm"><div className="flex min-h-7 items-center max-sm:min-h-11"><label htmlFor={id}>{t(field.label)}</label>{field.required && <span className="ml-1 text-danger" aria-hidden="true">*</span>}{field.help && <FieldHelp title={t(field.label)}>{t(field.help)}</FieldHelp>}</div>
    {field.kind === "select" ? <select id={id} aria-invalid={invalid || undefined} className={base} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)}><option value="">{t("请选择")}</option>{field.options?.map(option => <option key={option.value} value={option.value}>{t(option.label)}</option>)}</select> : ["textarea", "json"].includes(field.kind) ? <textarea id={id} aria-invalid={invalid || undefined} className={`${base} min-h-28 ${field.kind === "json" ? "font-mono" : ""}`} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)} spellCheck={field.kind !== "json"} /> : field.kind === "file" ? <Input id={id} aria-invalid={invalid || undefined} className="mt-2" type="file" required={field.required} disabled={disabled} onChange={event => { const file = event.target.files?.[0]; if (file) onFile(file); }} /> : <Input id={id} aria-invalid={invalid || undefined} className="mt-2" type={field.kind === "email" ? "email" : field.kind === "password" ? "password" : field.kind === "datetime" ? "datetime-local" : "text"} inputMode={field.kind === "decimal" ? "decimal" : field.kind === "integer" ? "numeric" : undefined} autoComplete={field.kind === "password" ? "new-password" : "off"} minLength={field.name === "initial_password" ? 10 : undefined} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)} />}

  </div>;
}

function PermissionField({ field, value, disabled, change }: { field: CommandField; value: string; disabled: boolean; change: (value: string) => void }) {
  const { t } = useI18n();
  const selected = value.split(",").map(item => item.trim()).filter(Boolean);
  const options = [...(field.options || []), ...selected.filter(item => !field.options?.some(option => option.value === item)).map(item => ({ value: item, label: field.name === "grants" ? t("现有扩展权限 · {permission}", { permission: item }) : t("已选择的现有人员") }))];
  return <fieldset className="text-sm"><legend>{t(field.label)}{field.help && <FieldHelp title={t(field.label)}>{t(field.help)}</FieldHelp>}</legend><div className="mt-2 grid gap-2 sm:grid-cols-2">{options.map(option => <label key={option.value} className="flex min-h-11 items-center gap-2 rounded-control border border-hairline px-3 py-2"><input type="checkbox" aria-label={t(option.label)} checked={selected.includes(option.value)} disabled={disabled} onChange={event => change((event.target.checked ? [...selected, option.value] : selected.filter(item => item !== option.value)).join(","))} />{t(option.label)}</label>)}</div></fieldset>;
}

const ReferenceSchema = z.object({ items: z.array(z.object({ value: z.string(), label: z.string(), defaults: z.record(z.string(), z.unknown()).optional() })), next_cursor: z.string().optional() });
function ReferenceField({ field, value, disabled, invalid, change, choose, onLabel, primary }: { field: CommandField; value: string; disabled: boolean; invalid: boolean; change: (value: string) => void; choose: (values: Record<string, unknown>) => void; onLabel: (label: string) => void; primary: boolean }) {
  const { t } = useI18n();
  const [search, setSearch] = useState(""); const deferred = useDeferredValue(search); const [cursor, setCursor] = useState(""); const [selectedLabel, setSelectedLabel] = useState(""); const path = field.lookup_url!; const separator = path.includes("?") ? "&" : "?";
  const data = useQuery({ queryKey: ["references", path, deferred, cursor], queryFn: ({ signal }) => request(`${path}${separator}q=${encodeURIComponent(deferred)}&cursor=${encodeURIComponent(cursor)}`, ReferenceSchema, { signal }) });
  const selected = useQuery({ queryKey: ["reference-label", path, value], queryFn: ({ signal }) => request(`${path}${separator}q=${encodeURIComponent(value)}`, ReferenceSchema, { signal }), enabled: !!value && !data.data?.items.some(item => item.value === value) });
  const currentLabel = data.data?.items.find(item => item.value === value)?.label || selected.data?.items.find(item => item.value === value)?.label || selectedLabel;
  useEffect(() => { onLabel(currentLabel); }, [currentLabel, onLabel]);
  function select(next: string) {
    change(next);
    const option = data.data?.items.find(item => item.value === next);
    setSelectedLabel(option?.label || "");
    onLabel(option?.label || "");
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
  const options: RecordChoice[] = (data.data?.items || []).map(item => ({ value: item.value, label: item.label, defaults: item.defaults }));
  const shown = !value ? "" : currentLabel || t(selected.isPending ? "正在读取关联记录名称…" : "已选择关联记录");
  return <div className="min-w-0 text-sm"><div className="flex min-h-7 items-center max-sm:min-h-11"><span>{t(field.label)}</span>{field.required && <span className="ml-1 text-danger" aria-hidden="true">*</span>}{field.help && <FieldHelp title={t(field.label)}>{t(field.help)}</FieldHelp>}</div>
    <RecordPicker label={t(field.label)} required={field.required} invalid={invalid} disabled={disabled} value={value} displayLabel={shown} options={options} placeholder={t("按名称或编号搜索")} emptyLabel={field.required ? t("请选择实际业务记录") : primary ? t("新建 / 不选择现有记录") : t("不选择关联记录")} loading={data.isPending} error={data.isError ? data.error : undefined} onRetry={() => void data.refetch()} moreLabel={data.data?.next_cursor ? t("更多记录") : undefined} onMore={data.data?.next_cursor ? () => setCursor(data.data!.next_cursor!) : undefined} onQuery={next => { setSearch(next); setCursor(""); }} onChoose={choice => select(choice?.value || "")} />
  </div>;
}
