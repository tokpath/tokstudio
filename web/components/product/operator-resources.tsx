"use client";
import { BusinessSettings } from "./business-settings";
import { useMonetary } from "./monetary";
import { MarketingCard, ReferralPolicyCard } from "./marketing-cards";
import { LanguageControl, useI18n } from "@/components/i18n";
import { translate, type Locale } from "@/lib/i18n";

import { createContext, useCallback, useContext, useEffect, useId, useState, type ReactNode } from "react";
import Link from "next/link";
import * as Dialog from "@radix-ui/react-dialog";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { pricingRefetchInterval } from "./pricing-refresh";
import { X, ArrowUpRight, Search, SlidersHorizontal } from "lucide-react";
import { request } from "@/lib/api-client";
import { CommandResultSchema, CreditsSchema, ResourcePageSchema, type Entity, type Operation, type JSONValue, type ResourcePage } from "@/lib/contracts";
import { canReadResource, resourceLabel } from "@/lib/navigation";
import { useNavigationBlock } from "@/lib/navigation-guard";
import { useProduct } from "./context";
import { InvoiceFiles } from "./invoice-files";
import { OperationForm } from "./operation-form";
import { Empty, ErrorPanel, Heading, Loading, Money, Notice, Stat, Status, Table, useCommand, When, statusLabels } from "./shared";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PurchaseTierSummary } from "./purchase-tiers";
import { FinanceEvidenceLinks } from "./finance-evidence";
import { ProcurementCards } from "./procurement-cards";
import { ModelPricingCard, PackageCard, priceDraft } from "./pricing-cards";
import { Card } from "@/components/ui/card";
import { readResourceListState, updateResourceListURL, subjectURL, type ResourceFilter } from "./operator-list-state";
import { FieldHelp } from "./field-help";
import { businessFieldHelp } from "./business-field-help";
import { readableResourceValue, resourceColumnVisible } from "@/lib/resource-presentation";
import { OperatorNotificationGroups } from "./operator-notifications";

export type InspectResource = (resource: string, row: Entity) => void;
const PanelLockContext = createContext<((id: string, blocked: boolean) => void) | undefined>(undefined);

// A nested operation must also keep every owning panel mounted while its
// original financial attempt is in flight or awaiting a reliable result.
function useParentPanelLock(blocked: boolean) {
  const update = useContext(PanelLockContext);
  const id = useId();
  useEffect(() => { update?.(id, blocked); return () => update?.(id, false); }, [update, id, blocked]);
}
type ResourceOptions = {
  operatorID?: string;
  resource: string;
  customer?: boolean;
  embedded?: boolean;
  layout?: "table" | "cards";
  onInspect?: InspectResource;
  record?: string;
  inspectionLinks?: boolean;
  businessView?: string;
  procurementSection?: string;
  onProcurementSection?: (section:string)=>void;
};

export function ResourceScreen({ operatorID, resource, customer = false, embedded = false, layout = "table", onInspect, record, inspectionLinks }: ResourceOptions) {
  const { t } = useI18n();
  const cache = useQueryClient();
  const { session } = useProduct();
  const [filters, setFilters] = useState(readResourceListState);
  const [query, setQuery] = useState(filters.q);
  const searchDescription = useId();
  const scope = operatorID ? `/api/v1/operators/${encodeURIComponent(operatorID)}` : "/api/v1/me";
  const resourceName = resource.split("/")[0];
  useEffect(() => { updateResourceListURL(filters); }, [filters]);
  const parameters = new URLSearchParams();
  if (filters.cursor) parameters.set("cursor", filters.cursor);
  if (filters.q) parameters.set("q", filters.q);
  if (filters.view) parameters.set("view", filters.view);
  if (filters.status) parameters.set("status", filters.status);
  if (record) parameters.set("record", record);
  else if(resourceName === "procurement") parameters.set("section",filters.section || "orders");
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const canRead = customer ? session?.kind === "customer" : canReadResource(workspace, resourceName);
  const keyFor = (state: typeof filters) => ["resource", scope, resource, state.cursor, state.q, state.view, state.status, state.section, record || ""];
  const data = useQuery({ queryKey: keyFor(filters), queryFn: ({ signal }) => request(`${scope}/${resource}${parameters.size ? `?${parameters}` : ""}`, ResourcePageSchema, { signal }), enabled: canRead, refetchInterval: query => resourceName === "pricing" ? pricingRefetchInterval(query.state.data) : false });
  if (!canRead) return <Empty title={t("没有查看权限")}>{t("此页面需要当前经营方明确授予相应权限。")}</Empty>;
  if (data.isPending) return <Loading />;
  if (data.isError) return <ErrorPanel error={data.error} retry={() => void data.refetch()} />;
  const page = data.data;
  const serverSearch = page.search_supported === true;
  const statusFilter = page.status_supported === true;
  const apply = (next: Partial<ResourceFilter>) => {
    const updated = { ...filters, ...next, cursor: "", history: [] };
    // Explicit searches must read again, including unchanged conditions and a
    // previously cached target page. Invalidate only that target, not old filters.
    void cache.invalidateQueries({ queryKey: keyFor(updated), exact: true });
    setFilters(updated);
  };
  const statuses = [...new Set([...(resourceName==="procurement" ? (filters.section==="quotes" ? ["quote","disabled","expired","archived"] : filters.section==="automatic" ? ["authorization","prepared","awaiting_manual_payment","outcome_unknown","verified_payment_delivery_pending","completed","failed_unpaid","cancelled_unpaid"] : ["awaiting_payment","paid","completed","closed","exception"]) : resourceFilterStatuses[resourceName] || []), ...page.items.flatMap(item => item.status ? [item.status] : []), filters.status].filter(Boolean))];
  return <>
    {record && <div className="mb-5"><Notice><div className="flex flex-wrap items-center justify-between gap-3"><span>{t("正在办理关联事项")}</span><Link className="text-brand-emphasis underline" href={operatorID ? `/manage/${encodeURIComponent(operatorID)}/${resource}` : `/workspace/${resource}`}>{t("返回全部记录")}</Link></div></Notice></div>}
    {page.views && <div aria-label={t("业务视图")} className="mb-5 flex flex-wrap gap-2">{page.views.map(item => <Button key={item.value} variant={(filters.view || page.views?.[0]?.value) === item.value ? "default" : "outline"} onClick={() => apply({ view: item.value, ...(resourceName==="procurement" ? {section:"orders",status:""} : {}) })}>{t(item.label)}</Button>)}</div>}
    <ResourceCollection page={page} operatorID={operatorID} resource={resource} customer={customer} embedded={embedded} layout={layout} onInspect={onInspect} inspectionLinks={inspectionLinks} businessView={filters.view || page.views?.[0]?.value} procurementSection={record ? (page.items[0]?.values.record_kind==="quote" ? "quotes" : ["authorization","replenishment_attempt"].includes(String(page.items[0]?.values.record_kind)) ? "automatic" : "orders") : filters.section || "orders"} onProcurementSection={section=>apply({section,status:""})} record={record} toolbar={serverSearch || statusFilter ? <div className="min-w-0 flex-1 basis-[32rem]"><form onSubmit={event => { event.preventDefault(); apply({ q: query.trim() }); }}>
      <div className="flex flex-wrap items-center gap-2">{serverSearch && <div className="relative min-w-48 flex-1"><Search size={15} aria-hidden className="absolute left-3 top-3.5 text-ink-mute" /><Input aria-label={t("搜索记录")} aria-describedby={searchDescription} placeholder={resourceName === "notifications" ? t("搜索通知标题、正文或关联事项") : resourceName === "customers" ? t("搜索客户名称或邮箱") : resourceName === "invoices" ? t("搜索发票编号、开票凭证或原始收款编号") : t("搜索业务编号、名称或关联记录")} value={query} onChange={event => setQuery(event.target.value)} className="pl-9" /></div>}
      {statusFilter && <label className="flex items-center gap-2 text-xs text-ink-secondary"><span>{t("状态")}</span><select className="min-h-11 rounded-control border border-hairline bg-canvas-raised px-3 text-sm" aria-label={t("筛选状态")} value={filters.status} onChange={event => apply({ status: event.target.value })}><option value="">{t("全部状态")}</option>{statuses.map(status => <option key={status} value={status}>{t(resourceFilterStatusLabel(resourceName, status))}</option>)}</select></label>}
      {serverSearch && <Button variant="outline" type="submit" disabled={data.isPending} aria-busy={data.isFetching}>{t("查询")}</Button>}{(serverSearch && filters.q || statusFilter && filters.status) && <Button variant="ghost" type="button" onClick={() => { setQuery(""); apply({ q: "", status: "" }); }}>{t("清除")}</Button>}</div>
      {serverSearch && <p id={searchDescription} className="mt-2 text-xs leading-6 text-ink-mute">{t("在当前权限范围内检索记录，结果按当前列表规则分页。")}</p>}
    </form></div> : undefined} />
    {resourceName !== "business-status" && <div className="mt-5 flex items-center justify-between gap-3"><p className="text-xs text-ink-mute">{t("当前页 {count} 条", { count: page.items.length })}{page.next_cursor ? t(" · 还有后续记录") : ""}</p><div className="flex gap-2">{filters.history.length > 0 && <Button variant="outline" onClick={() => setFilters(current => { const history = [...current.history]; return { ...current, cursor: history.pop() || "", history }; })}>{t("上一页")}</Button>}{page.next_cursor && <Button variant="outline" onClick={() => setFilters(current => ({ ...current, history: [...current.history, current.cursor], cursor: page.next_cursor || "" }))}>{t("下一页")}</Button>}</div></div>}
  </>;
}

// Stable choices come from persisted domain enums, so a status remains
// selectable when every matching record is beyond the currently loaded page.
const resourceFilterStatuses: Record<string, readonly string[]> = {
  customers: ["active", "stopped", "closing", "closed", "anonymized", "suspended"],
  orders: ["pending", "awaiting_payment", "partial", "paid", "completed", "closed", "exception"],
  payments: ["pending", "confirmed"],
  notifications: ["pending", "processing", "accepted", "needs_attention", "suppressed"],
  procurement: ["quote","disabled","expired","archived","awaiting_payment","paid","completed","closed","exception","authorization","prepared","awaiting_manual_payment","outcome_unknown","verified_payment_delivery_pending","failed_unpaid","cancelled_unpaid"],
  refunds: ["pending_review", "approved", "paying", "completed", "rejected", "cancelled", "exception"],
  withdrawals: ["requested", "approved", "paying", "unknown", "paid", "cancelled", "failed"],
  invoices: ["requested", "issued", "rejected", "review_required", "voided"],
  "procurement-invoices": ["requested", "issued", "rejected", "review_required", "voided"],
  "receipt-valuations": ["awaiting_settlement"],
  assets: ["open"], ledger: ["open"], gifts: ["delivered"],
  jobs: ["pending", "running", "completed", "dead"],
};
function resourceFilterStatusLabel(resource: string, status: string): string {
  if (resource === "customers" && status === "suspended") return "已暂停账号";
  return ({ partial: "部分付款", anonymized: "已匿名", open: "正常" } as Record<string, string>)[status] || statusLabels[status] || status;
}

const publicCapabilityOrder = ["chat", "responses", "embedding", "image_generation", "speech", "transcription", "video_generation"];
const publicCapabilityLabels: Record<string, string> = { chat: "对话", responses: "Responses", embedding: "向量", image_generation: "图片生成", speech: "语音生成", transcription: "语音转写", video_generation: "视频生成" };
function PublicModelCapabilities({ value }: { value: JSONValue | undefined }) {
  const { t } = useI18n();
  const selected = new Set<string>();
  if (Array.isArray(value)) for (const item of value) {
    const operation = item && typeof item === "object" && !Array.isArray(item) && typeof item.operation === "string" ? item.operation : typeof item === "string" ? item : "";
    if (operation) selected.add(operation);
  }
  const labels = [...publicCapabilityOrder.filter(operation => selected.has(operation)), ...[...selected].filter(operation => !publicCapabilityOrder.includes(operation))].map(operation => t(publicCapabilityLabels[operation] || operation));
  return labels.length ? <span>{labels.join("、")}</span> : <span className="text-ink-mute">—</span>;
}

function PublicModelSwitch({ row, operatorID }: { row: Entity; operatorID: string }) {
  const { t } = useI18n();
  const saved = row.values.enabled === true;
  const [pending, setPending] = useState<boolean>();
  const open = pending ?? saved;
  const routed = row.values.route_configured === true;
  const blocked = !open && !routed;
  const command = useCommand(`/api/v1/operators/${encodeURIComponent(operatorID)}/catalog/${encodeURIComponent(row.id)}/availability`, CommandResultSchema);
  const message = command.error instanceof Error ? command.error.message : "";
  useEffect(() => { setPending(undefined); }, [saved, row.version]);
  return <div className="flex min-w-0 flex-col items-start gap-1">
    <button type="button" role="switch" aria-checked={open} aria-label={t("开放模型")} disabled={blocked || command.busy} title={blocked ? t("还没有配置路由，不能开放") : undefined} onClick={() => { const next = !open; setPending(next); void command.run({ enabled: next, expected_version: row.version ?? 0 }).then(result => { if (!result) setPending(undefined); }); }} className={`relative h-6 w-11 shrink-0 rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${open ? "bg-brand" : "bg-hairline"}`}>
      <span aria-hidden className={`absolute top-0.5 left-0.5 size-5 rounded-full bg-white shadow transition-transform ${open ? "translate-x-5" : ""}`} />
    </button>
    {blocked && <span className="max-w-40 text-xs leading-5 text-ink-mute">{t("还没有配置路由，不能开放")}</span>}
    {!!message && <span role="alert" className="max-w-48 text-xs leading-5 text-danger">{t(message)}</span>}
  </div>;
}

function CustomerWithdrawalBar({ minimum }: { minimum?: { value: string; unit?: string; detail?: string } }) {
  const { t } = useI18n();
  const credits = useQuery({ queryKey: ["credits"], queryFn: () => request("/api/v1/me/credits", CreditsSchema) });
  const wallet = credits.data?.commission_money;
  const amounts = [
    { label: "可用金额", value: wallet?.available, money: true },
    { label: "冻结金额", value: wallet?.frozen, money: true },
    { label: "最低提现额度", value: minimum?.value, money: minimum?.unit === "MONEY" },
  ];
  return <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-8 gap-y-3">{amounts.map(item => <div key={item.label} className="min-w-0"><p className="text-xs text-ink-mute">{t(item.label)}</p>{item.money && item.value ? <p className="mt-1 text-lg font-semibold"><Money value={item.value} unit="MONEY" /></p> : <p className="mt-1 text-lg font-semibold">{item.value ? t(item.value) : t("尚未读取")}</p>}</div>)}<p className="w-full text-xs leading-6 text-ink-mute">{t("提现手续费由你承担。经营方单人审批后付款。")}</p></div>;
}
export function ResourceCollection({ page, operatorID, resource, customer = false, embedded = false, layout = "table", onInspect, toolbar, initialValues, actionsOnly = false, record, inspectionLinks, businessView, procurementSection, onProcurementSection }: ResourceOptions & { page: ResourcePage; toolbar?: ReactNode; initialValues?: Record<string, JSONValue>; actionsOnly?: boolean }) {
  const { t, locale } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const [selection, setSelection] = useState<{ operation: Operation; row?: Entity }>();
  const [blocked, setBlocked] = useState(false), [invoiceID, setInvoiceID] = useState(""), [facts, setFacts] = useState<Entity>();
  const [recordKind, setRecordKind] = useState("");
  const [localProcurementTab,setLocalProcurementTab]=useState("orders");
  const procurementTab=procurementSection || localProcurementTab;
  const setProcurementTab=(next:string)=>{setLocalProcurementTab(next);onProcurementSection?.(next);};
  const resourceName = resource.split("/")[0];
  const canOperate = (operation: Operation) => !operation.permission || !!workspace?.permissions.includes("*") || !!workspace?.permissions.includes(operation.permission);
  const operations = page.operations.filter(operation => canOperate(operation) && (resourceName !== "procurement" || (operation.name === "quote" ? procurementTab === "quotes" : operation.name === "authorization" ? procurementTab === "automatic" : true)));
  const settings = operations.filter(operation => ["refunds","withdrawals","invoices"].includes(resourceName) && (operation.name === "policy" || operation.name.startsWith("invoice_policy_")));
  const primaryOperations = operations.filter(operation => !settings.includes(operation));
  const editable = editableOperations[resourceName];
  const editOperation = editable && operations.find(operation => operation.name === editable.name);
  const rowOperations = (row: Entity) => page.row_operations.filter(operation => canOperate(operation) && (!row.allowed_actions || row.allowed_actions.includes(operation.name)));
  const closeOperation = () => { if (!blocked) setSelection(undefined); };
  const selectOperation = (operation: Operation, row?: Entity) => setSelection({ operation, row: row || (currentRecordOperations[resourceName]?.includes(operation.name) ? page.items[0] : undefined) });
  const savedValues: Record<string, JSONValue> = selection?.row && ["branding", "channel-themes"].includes(resourceName) ? { values: selection.row.values } : {};
  if (selection?.row && resourceName === "coverage") {
    if (selection.row.values.oversell !== undefined) savedValues.allowance = selection.row.values.oversell;
    if (selection.row.values.warning_buffer !== undefined) savedValues.warning = selection.row.values.warning_buffer;
  }
  const scope = operatorID ? `/api/v1/operators/${encodeURIComponent(operatorID)}` : "/api/v1/me";
  const inspect = (row: Entity) => onInspect && ["customers", "orders"].includes(resourceName) ? onInspect(resourceName, row) : setFacts(row);
  const showInspect = resourceName !== "catalog" && resourceName !== "providers";
  const inspectLabel = resourceName === "customers" ? t("客户工作台") : resourceName === "orders" ? t("查看并办理") : t("查看详情");
  const kinds = [...new Set(page.items.map(row => row.values.record_kind).filter((value): value is string => typeof value === "string"))];
  const procurementGroups: Record<string, string[]> = { orders: ["order"], quotes: ["quote"], automatic: ["authorization", "replenishment_attempt"] };
  const rows = resourceName === "procurement" ? page.items.filter(row => procurementGroups[procurementTab].includes(String(row.values.record_kind))) : recordKind ? page.items.filter(row => row.values.record_kind === recordKind) : page.items;
  const columnsFor = (items: Entity[]) => page.columns.filter(column => resourceColumnVisible(column.key, resourceName) && !["id", "status", "name", "display_name", "record_kind"].includes(column.key) && items.some(row => hasResourceValue(row.values[column.key])));
  const populatedColumns = columnsFor(rows);
  const procurementHold = (row: Entity) => resourceName === "procurement" && ["awaiting_payment", "paid"].includes(row.status || "");
  const identity = (row: Entity) => <div className="min-w-32"><p className="font-medium">{recordTitle(row, resourceName, locale)}</p>{row.presentation?.subtitle && <p className="mt-1 text-xs text-ink-mute">{row.presentation.subtitle}</p>}</div>;
  const renderTable = (items: Entity[]) => resourceName === "procurement" ? <ProcurementCards rows={items} title={row => recordTitle(row, resourceName, locale)} actions={actions} /> : resourceName === "supplier-costs" ? <Table rows={items} getID={row => row.id} columns={[
    {label:"供应商与期间",cell:row=><div className="min-w-48 max-w-72"><p className="font-medium">{String(row.values.provider_name || "")}</p><p className="mt-1 text-xs text-ink-secondary">{row.values.model_name === "供应商整体" ? t("供应商整体") : String(row.values.model_name || "")}</p><p className="mt-2 text-xs text-ink-mute"><When value={String(row.values.starts_at)} /> — <When value={String(row.values.ends_at)} /></p>{row.values.note && <p className="mt-2 line-clamp-2 text-xs leading-5 text-ink-secondary">{String(row.values.note)}</p>}</div>},
    {label:t("供应商参考成本"),align:"right",cell:(row:Entity)=><div className="min-w-24"><Money value={String(row.values.reference_amount || "0")} unit={String(row.values.reference_currency || "USD")} />{Number(row.values.pending_count)>0 && <p className="mt-2 text-xs text-hold">{t("{count} 次调用缺少核算金额",{count:Number(row.values.pending_count)})}</p>}</div>},
    {label:t("供应商实际账单"),align:"right",cell:(row:Entity)=><Money value={String(row.values.amount || "0")} unit={String(row.values.currency || "")} />},
    {label:"办理",cell:row=><div className="min-w-28">{actions(row)}</div>},
  ]} /> : <Table rows={items} getID={row => row.id} empty={t("当前筛选下没有记录。可查询其他编号或继续办理新事项。")} columns={[{ label: t("名称与事项"), cell: identity }, ...columnsFor(items).filter(column => column.key !== "state").map(column => ({ label: column.label, help: businessFieldHelp(`${resourceName}.${column.key}`, column.help), align: ["money", "atg", "decimal"].includes(column.kind) ? "right" as const : undefined, cell: (row: Entity) => { if ((resourceName === "catalog" || resourceName === "providers") && column.key === "capabilities") return <PublicModelCapabilities value={row.values.capabilities} />; if (resourceName === "catalog" && column.key === "enabled" && operatorID) return <PublicModelSwitch row={row} operatorID={operatorID} />; const value = <ResourceValue value={readableResourceValue(row, column.key, locale)} kind={column.kind} field={column.key} settlementCurrency={recordSettlementCurrency(row)} />; if (column.key === "reserved_atg" && procurementHold(row)) return <span className="inline-flex rounded-md bg-danger/10 px-2 py-1 font-semibold text-danger">{value}</span>; if (column.key === "reserved_note" && procurementHold(row)) return <span className="font-medium text-danger">{value}</span>; return value; } })), { label: t(resourceName === "notifications" ? "邮件状态" : "状态"), cell: (row: Entity) => row.status ? <Status value={row.status} /> : <span>—</span> }, { label: t("办理"), cell: (row: Entity) => actions(row) }]} />;
  const renderRefund = (row: Entity, children: Entity[]) => <section key={row.id} className="rounded-card border border-hairline bg-canvas-raised p-5"><div className="flex flex-wrap items-start justify-between gap-3">{identity(row)}{row.status && <Status value={row.status} />}</div><div className="my-4 grid gap-4 sm:grid-cols-3"><div><p className="mb-1 text-xs text-ink-mute">{t("退款责任")}</p><ResourceValue value={row.values.amount_money} kind="money" /></div><div><p className="mb-1 text-xs text-ink-mute">{t("已真实清偿")}</p><ResourceValue value={sumResourceAmounts(children, "settled_money")} kind="money" /></div><div><p className="mb-1 text-xs text-ink-mute">{t("仍待付款")}</p><ResourceValue value={sumResourceAmounts(children, "remaining_money")} kind="money" /></div></div><p className="mb-4 text-sm text-ink-secondary">{t(refundNextStep(row, children))}</p>{actions(row)}{children.length > 0 && <details className="mt-5 border-t border-hairline pt-4" open={!!record}><summary className="cursor-pointer text-sm text-brand-emphasis">{t("关联退款明细与付款")} · {children.length}</summary><div className="mt-4 space-y-4">{children.map(leg => <div key={leg.id} className="rounded-control border border-hairline p-4"><div className="mb-3 flex flex-wrap items-center justify-between gap-3">{identity(leg)}{leg.status && <Status value={leg.status} />}</div><div className="mb-4 grid gap-3 sm:grid-cols-3">{["amount_money", "settled_money", "remaining_money"].map(key => <div key={key}><p className="mb-1 text-xs text-ink-mute">{t(page.columns.find(column => column.key === key)?.label || key)}</p><ResourceValue value={leg.values[key]} kind="money" /></div>)}</div>{actions(leg)}{page.items.filter(payment => payment.values.record_kind === "actual_refund_payment" && payment.values.leg_id === leg.id).map(payment => <div key={payment.id} className="mt-4 border-l-2 border-hairline pl-4"><div className="mb-3 flex flex-wrap items-center justify-between gap-2">{identity(payment)}{payment.status && <Status value={payment.status} />}</div><p className="mb-3 text-xs text-ink-secondary">{t("本次实际付款")} <ResourceValue value={payment.values.actual_payment_money ?? payment.values.actual_money} kind="money" /> · {t("手续费")} <ResourceValue value={payment.values.fee_money} kind="money" /></p>{actions(payment)}</div>)}</div>)}</div></details>}</section>;
  const refundCases = resourceName === "refunds" ? rows.filter(row => row.values.record_kind === "case") : [];
  const linkedRefundIDs = new Set(refundCases.flatMap(row => { const legs = rows.filter(leg => leg.values.record_kind === "leg" && leg.values.case_id === row.id); return [row.id, ...legs.map(leg => leg.id), ...rows.filter(payment => legs.some(leg => payment.values.leg_id === leg.id)).map(payment => payment.id)]; }));
  const orphanRefunds = rows.filter(row => !linkedRefundIDs.has(row.id));
  const actions = (row: Entity) => <div className="flex flex-wrap items-center gap-1 text-brand-emphasis">
    {showInspect && (inspectionLinks && operatorID && ["customers", "orders"].includes(resourceName) ? <Button asChild variant="ghost" size="sm"><Link href={subjectURL(operatorID, resourceName, row.id, typeof window !== "undefined" ? window.location.pathname + window.location.search : undefined)} onClick={event => { if (onInspect && !event.ctrlKey && !event.metaKey && !event.shiftKey && event.button === 0) { event.preventDefault(); inspect(row); } }}>{inspectLabel}<ArrowUpRight size={13} aria-hidden className="ml-1" /></Link></Button> : <Button type="button" variant="ghost" size="sm" onClick={() => inspect(row)}>{inspectLabel}<ArrowUpRight size={13} aria-hidden className="ml-1" /></Button>)}{editOperation && (!row.allowed_actions || row.allowed_actions.includes(editOperation.name)) && <Button type="button" variant="ghost" size="sm" onClick={() => selectOperation({ ...editOperation, label: editable.label }, row)}>{t(editable.label)}
    </Button>}{rowOperations(row).map(operation => <Button key={operation.name} type="button" variant="ghost" size="sm" onClick={() => selectOperation(operation, row)}>{t(operation.label)}
    </Button>)}{["invoices", "procurement-invoices"].includes(resourceName) && <Button type="button" variant="ghost" size="sm" onClick={() => setInvoiceID(row.id)}>{t("文件与下载")}</Button>}
  </div>;
  if (resourceName === "business-status" && operatorID) return <>{!embedded && <Heading title={page.title} description={page.description} />}<BusinessSettings operatorID={operatorID} row={page.items[0]} /></>;
  return <>
    {!embedded && <Heading title={page.title === resourceName ? resourceLabel(resourceName) : page.title} description={page.description} />}
    {page.summary && !(customer && resourceName === "withdrawals") && !["refunds","invoices"].includes(resourceName) && (resourceName !== "procurement" || procurementTab === "orders") && <div className="mb-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{page.summary.map((item, index) => <div key={index}><Stat {...item} detail={undefined} help={item.detail} /></div>)}
    </div>}
    {page.availability?.filter(item => !item.enabled).map(item => <div key={item.capability} className="mb-4">
      <Notice>{t(item.reason || "相应业务暂未开放")}{item.next_action && <p>{t(item.next_action)}
      </p>}
      </Notice>
    </div>)}
    {resourceName === "procurement" && <div className="mb-5"><div role="tablist" aria-label={t("采购办理事项")} className="flex flex-wrap gap-2">{[["orders", businessView === "supplier" ? "供货订单" : "采购订单"], ["quotes", businessView === "supplier" ? "供货报价" : "采购报价"], ["automatic", "自动补货"]].filter(([key]) => key !== "automatic" || businessView !== "supplier").map(([key, title]) => <Button key={key} role="tab" aria-selected={procurementTab === key} variant={procurementTab === key ? "default" : "outline"} onClick={() => setProcurementTab(key)}>{t(title)}</Button>)}</div><p className="mt-3 text-xs leading-6 text-ink-mute">{t(businessView === "supplier" ? "供货订单列出每一笔采购占用的库存。未付款订单不限时，直到付清或取消后才释放。" : procurementTab === "quotes" ? "选择已生效报价创建采购订单。创建后即占用供货库存，付款并交付后才增加采购余额。" : procurementTab === "automatic" ? "在此管理有限补货授权和原执行结果。结果未知时只查询原付款。" : "每一行显示这笔订单占用的供货库存。未付款不限时，直到付清或取消。需要新增采购时打开采购报价。")}</p></div>}
    {operatorID && ["procurement", "operators"].includes(resourceName) && workspace?.permissions.some(permission => ["*", "cooperation.manage"].includes(permission)) && <p className="mb-4 text-sm"><Link className="text-brand-emphasis underline" href={`/manage/${encodeURIComponent(operatorID)}/contracts`}>{t("上传或查看可选合作合同")}</Link></p>}
    {resourceName !== "providers" && (toolbar || operations.length > 0) && <div className="th-toolbar">{customer && resourceName === "withdrawals" && <CustomerWithdrawalBar minimum={page.summary?.find(item => item.label === "最低提现额度")} />}{toolbar}
      <div className={`${customer && resourceName === "withdrawals" ? "ml-auto " : ""}flex flex-wrap items-center gap-2`}>{primaryOperations.map((operation, index) => <Button key={operation.name} type="button" variant={index === 0 ? "default" : "outline"} onClick={() => selectOperation(operation)}>{t(operation.label)}
      </Button>)}
      {settings.length>0 && <details className="relative"><summary className="cursor-pointer rounded-control border border-hairline bg-canvas-raised px-4 py-3 text-sm">{t("办理设置")}</summary><div className="absolute right-0 z-20 mt-2 min-w-56 space-y-1 rounded-control border border-hairline bg-canvas-raised p-2 shadow-lg">{settings.map(operation => <Button key={operation.name} type="button" variant="ghost" className="w-full justify-start" onClick={() => selectOperation(operation)}>{t(operation.label)}</Button>)}</div></details>}
      </div>
    </div>}
    {resourceName === "providers" && toolbar && <div className="th-toolbar">{toolbar}</div>}
    {kinds.length > 1 && !["procurement", "refunds", "providers"].includes(resourceName) && <div className="mb-5 flex flex-wrap items-center gap-2 text-xs text-ink-mute">
      <SlidersHorizontal size={14} aria-hidden />
      <span>{t("本页类型")}</span>{["", ...kinds].map(kind => <Button key={kind} size="sm" variant={recordKind === kind ? "default" : "ghost"} onClick={() => setRecordKind(kind)}>{kind ? recordKindLabel(kind, locale) : t("全部")}
      </Button>)}
    </div>}
    {actionsOnly ? <div className="flex flex-wrap gap-2">{rows.flatMap(row => rowOperations(row).map(operation => <Button key={`${row.id}:${operation.name}`} type="button" variant="outline" onClick={() => setSelection({ operation, row })}>{t(operation.label)}
    </Button>))}
    </div> : resourceName === "procurement" && procurementTab === "automatic" ? <div className="space-y-6"><section><h3 className="mb-3 text-sm font-semibold">{t("补货授权")}</h3>{renderTable(rows.filter(row => row.values.record_kind === "authorization"))}</section><section><h3 className="mb-3 text-sm font-semibold">{t("补货执行记录")}</h3>{renderTable(rows.filter(row => row.values.record_kind === "replenishment_attempt"))}</section></div> : refundCases.length > 0 ? <div className="space-y-5">{refundCases.map(row => renderRefund(row, rows.filter(leg => leg.values.record_kind === "leg" && leg.values.case_id === row.id)))}{orphanRefunds.length > 0 && <div><p className="mb-3 text-sm text-ink-secondary">{t("本页其他关联记录")}</p>{renderTable(orphanRefunds)}</div>}</div> : resourceName === "providers" ? <div className="space-y-10">{([["credential", "供应商密钥", "密钥加密保存，页面不再显示原文。更换时留空表示继续用原来的密钥。", "credential"], ["supplier_model", "供应商模型", "选择协议、供应商模型与能力；参考成本由 Bifrost 核算，客户与下级的 ATG 价格在模型定价中设置。", "model"]] as const).map(([kind, title, detail, action]) => <section key={kind}><div className="mb-4 flex flex-wrap items-end justify-between gap-3"><div><h2 className="text-lg font-semibold">{t(title)}</h2><p className="mt-1 max-w-3xl text-sm leading-6 text-ink-mute">{t(detail)}</p></div><div className="flex flex-wrap gap-2">{operations.filter(operation => operation.name === action).map(operation => <Button key={operation.name} type="button" onClick={() => selectOperation(action === "model" ? { ...operation, label: "添加供应商模型" } : operation)}>{t(action === "model" ? "添加供应商模型" : operation.label)}</Button>)}</div></div>{renderTable(page.items.filter(row => row.values.record_kind === kind))}</section>)}</div> : resourceName === "pricing" ? <div className="space-y-5">{rows.map(row => <ModelPricingCard key={row.id} row={row} canPrice={operations.some(operation => operation.name === "price")} canTemplate={operations.some(operation => operation.name === "template")} onPrice={price => { const operation = operations.find(operation => operation.name === "price"); if(operation) selectOperation(operation, { ...row, values: { ...row.values, configuration: priceDraft(row, price) } }); }} onTemplate={() => { const operation=operations.find(operation=>operation.name==="template"), billing=row.values.billing as Record<string,JSONValue> || {}, ability=Object.keys(billing)[0]; if(operation) selectOperation(operation, {...row,values:{...row.values,configuration:{action:"billing",model_id:row.id,operation:ability || "",billing:billing[ability] || {input:[],output:[]},expected_version:row.version || 0}}}); }} />)}{!rows.length && <Empty>{t("暂无已授权模型，请先完成公开模型与模型授权。")}</Empty>}</div> : resourceName === "plans" ? <div className="grid gap-4 xl:grid-cols-2">{rows.map(row => <PackageCard key={row.id} row={row}>{actions(row)}</PackageCard>)}{!rows.length && <Empty>{t("创建一个通用额度套餐即可开始准备销售。")}</Empty>}</div>  : resourceName === "marketing" ? <div className="grid gap-4 xl:grid-cols-2">{rows.map(row => <MarketingCard key={row.id} row={row} ownerName={row.values.owner_customer_id ? String(readableResourceValue(row, "owner_customer_id", locale) || "") : undefined} packageName={String(readableResourceValue(row, "package_id", locale) || "")}>{actions(row)}</MarketingCard>)}{!rows.length && <Empty>{t("还没有套餐活动，选择已发布套餐即可创建。")}</Empty>}</div> : resourceName === "commissions" && rows.some(row => row.values.direct_percent !== undefined) ? <div className="space-y-4">{rows.map(row => <ReferralPolicyCard key={row.id} row={row}>{actions(row)}</ReferralPolicyCard>)}</div> : resourceName === "notifications" && operatorID ? <OperatorNotificationGroups operatorID={operatorID} page={page} onRetry={noticeID => { const operation = page.row_operations.find(item => item.name === "retry"); if (operation) selectOperation(operation, { id: noticeID, values: {} }); }} /> : layout === "cards" ? <div className="grid gap-4 xl:grid-cols-2">{rows.map(row => <Card key={row.id} className="min-w-0">
      <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-base font-semibold">{recordTitle(row, resourceName, locale)}
          </h3>
          {row.presentation?.subtitle && <p className="mt-1 text-xs text-ink-mute">{row.presentation.subtitle}</p>}
        </div>{row.status && <Status value={row.status} />}
      </div>
      <dl className="mb-5 grid gap-x-5 gap-y-4 sm:grid-cols-2">{populatedColumns.filter(column => hasResourceValue(row.values[column.key])).slice(0, 6).map(column => <div key={column.key} className="min-w-0">
        <dt className="mb-1 text-xs text-ink-mute">{t(column.label)}{businessFieldHelp(`${resourceName}.${column.key}`, column.help) && <FieldHelp title={t(column.label)}>{t(businessFieldHelp(`${resourceName}.${column.key}`, column.help)!)}</FieldHelp>}
        </dt>
        <dd className="min-w-0 text-sm">
          <ResourceValue value={readableResourceValue(row, column.key, locale)} kind={column.kind} field={column.key} settlementCurrency={recordSettlementCurrency(row)} />
        </dd>
      </div>)}
      </dl>{actions(row)}
    </Card>)}{!rows.length && <div className="xl:col-span-2">
      <Empty>{t("当前筛选下没有记录。")}</Empty>
    </div>}
    </div> : renderTable(rows)}
    {selection && <OperatorPanel title={t(selection.operation.label)} description={selection.row ? recordTitle(selection.row, resourceName, locale) : customer ? t("在当前客户范围内办理") : t("在当前经营范围内办理")} open blocked={blocked} close={closeOperation}>
      <OperationForm key={`${selection.operation.name}:${selection.row?.id || "new"}`} operation={selection.operation} row={selection.row} operatorID={operatorID} embedded initialValues={{ ...savedValues, ...initialValues }} lockedFields={Object.keys(initialValues || {})} onBlockedChange={setBlocked} onAbandon={() => setSelection(undefined)} />
    </OperatorPanel>}
    {invoiceID && <InvoiceFiles scope={`${scope}/${resourceName}`} invoiceID={invoiceID} upload={!customer && resourceName === "invoices"} close={() => setInvoiceID("")} />}
    {facts && <OperatorPanel open title={recordTitle(facts, resourceName, locale)} description={t("记录 {id}", { id: facts.id })} close={() => setFacts(undefined)}>
      <div className="mb-5 space-y-3"><OriginalRecordID id={facts.id} />{facts.status && <Status value={facts.status} />}
      </div>
      <dl className="grid gap-5 sm:grid-cols-2">{page.columns.filter(column => column.key !== "id" && (column.key === "status" || hasResourceValue(facts.values[column.key]))).map(column => <div key={column.key} className="min-w-0">
        <dt className="mb-2 text-xs text-ink-mute">{t(column.label)}{businessFieldHelp(`${resourceName}.${column.key}`, column.help) && <FieldHelp title={t(column.label)}>{t(businessFieldHelp(`${resourceName}.${column.key}`, column.help)!)}</FieldHelp>}
        </dt>
        <dd>
          <ResourceValue value={column.key === "status" ? facts.status : readableResourceValue(facts, column.key, locale)} kind={column.kind} field={column.key} settlementCurrency={recordSettlementCurrency(facts)} />
        </dd>
      </div>)}
      </dl>
      <details className="mt-7 border-t border-hairline pt-5">
        <summary className="cursor-pointer text-sm text-brand-emphasis">{t("完整保存记录与证据")}</summary>
        <div className="mt-4">
          <FinanceEvidenceLinks value={facts.values} operatorID={operatorID} /><ReadFacts value={facts.values} />
        </div>
      </details>
      <div className="mt-6 flex flex-wrap gap-2">{rowOperations(facts).map(operation => <Button key={operation.name} variant="outline" onClick={() => { setFacts(undefined); setSelection({ operation, row: facts }); }}>{t(operation.label)}
      </Button>)}
      </div>
    </OperatorPanel>}
  </>;
}

function OriginalRecordID({ id }: { id: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  return <div className="flex flex-wrap items-center gap-2 text-xs"><span className="text-ink-mute">{t("原始记录编号")}</span><code className="break-all">{id}</code><Button type="button" variant="outline" size="sm" onClick={async () => { try { await navigator.clipboard.writeText(id); setCopied(true); } catch { setCopied(false); } }}>{t(copied ? "已复制" : "复制编号")}</Button></div>;
}

export function OperatorPage({ title, description, close, children, onBlockedChange }: { title: string; description: string; close: () => void; children: ReactNode; onBlockedChange?: (blocked: boolean) => void }) {
  const { t } = useI18n();
  const [locks, setLocks] = useState<Set<string>>(() => new Set());
  const updateLock = useCallback((id: string, locked: boolean) => setLocks(current => { const next = new Set(current); if (locked) next.add(id); else next.delete(id); return next; }), []);
  const locked = locks.size > 0;
  const navigationBlocked = useNavigationBlock(locked);
  useEffect(() => { onBlockedChange?.(locked); }, [locked, onBlockedChange]);
  return <PanelLockContext.Provider value={updateLock}><div><div className="mb-5 flex items-center justify-between gap-3"><Button variant="outline" disabled={locked} onClick={close}>{t("返回原列表与筛选")}</Button><LanguageControl /></div><Heading title={title} description={description} />{navigationBlocked && <div className="mb-5"><Notice>{t("本次办理尚未取得确定结果，请保持此面板并重试原操作；确认完成后再离开。")}</Notice></div>}{children}</div></PanelLockContext.Provider>;
}

function hasResourceValue(value: JSONValue | undefined) { return value !== undefined && value !== null && value !== "" && (!Array.isArray(value) || value.length > 0); }
function sumResourceAmounts(rows: Entity[], key: string): string | undefined {
  if (!rows.length || rows.some(row => typeof row.values[key] !== "string" || !/^\d+(?:\.\d+)?$/.test(String(row.values[key])))) return undefined;
  const values = rows.map(row => String(row.values[key]));
  const places = Math.max(...values.map(value => value.split(".")[1]?.length || 0));
  const sum = values.reduce((total, value) => { const [whole, fraction = ""] = value.split("."); return total + BigInt(whole + fraction.padEnd(places, "0")); }, 0n).toString().padStart(places + 1, "0");
  return places ? `${sum.slice(0, -places)}.${sum.slice(-places)}` : sum;
}
function refundNextStep(row: Entity, legs: Entity[]): string {
  if (row.status === "pending_review") return "下一步：完成审批与独立复核。";
  if (legs.some(leg => ["unknown", "executing"].includes(leg.status || ""))) return "下一步：查询原退款结果并核实实际付款，保留未知金额占用。";
  if (["completed", "cancelled", "canceled", "rejected"].includes(row.status || "")) return "此事项已结束，可核对原责任与实际付款证据。";
  return "下一步：展开关联明细，按每笔原收款的已授权动作办理。";
}

// The choice list is drawn on the page, outside the dialog. A modal dialog
// ignores clicks there, so choosing a record must not count as leaving.
function recordPickerEvent(event: { detail?: { originalEvent?: Event } }) {
  const target = event.detail?.originalEvent?.target;
  return target instanceof Element && !!target.closest("[data-record-picker-menu]");
}

export function OperatorPanel({ title, description, open, close, blocked = false, children, onBlockedChange }: { title: string; description: string; open: boolean; close: () => void; blocked?: boolean; children: ReactNode; onBlockedChange?: (blocked: boolean) => void }) {
  const { t } = useI18n();
  const [childLocks, setChildLocks] = useState<Set<string>>(() => new Set());
  const updateLock = useCallback((id: string, locked: boolean) => setChildLocks(current => {
    if (current.has(id) === locked) return current;
    const next = new Set(current);
    if (locked) next.add(id); else next.delete(id);
    return next;
  }), []);
  const locked = blocked || childLocks.size > 0;
  const navigationBlocked = useNavigationBlock(locked);
  useParentPanelLock(locked);
  useEffect(() => { onBlockedChange?.(locked); }, [locked, onBlockedChange]);
  return <PanelLockContext.Provider value={updateLock}><Dialog.Root open={open} onOpenChange={value => { if (!value && !locked) close(); }}>
    <Dialog.Portal>
      <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim backdrop-blur-[3px]" />
      <Dialog.Content className="th-operation-panel fixed inset-y-0 right-0 z-50 w-full overflow-y-auto border-l border-hairline p-5 focus:outline-none sm:max-w-4xl sm:p-8" onEscapeKeyDown={event => { if (locked) event.preventDefault(); }} onPointerDownOutside={event => { if (locked || recordPickerEvent(event)) event.preventDefault(); }} onInteractOutside={event => { if (locked || recordPickerEvent(event)) event.preventDefault(); }}>
        <div className="mb-6 flex items-start justify-between gap-5 border-b border-hairline pb-5">
          <div className="min-w-0">
            <Dialog.Title className="text-2xl font-semibold tracking-tight">{title}
            </Dialog.Title>
            <Dialog.Description className="mt-2 break-words text-sm leading-6 text-ink-mute">{description}
            </Dialog.Description>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <LanguageControl />
          <Button variant="ghost" size="icon" aria-label={t("关闭办理面板")} disabled={locked} onClick={close}>
            <X size={20} />
          </Button>
          </div>
        </div>{navigationBlocked && <div className="mb-5"><Notice>{t("本次办理尚未取得确定结果，请保持此面板并重试原操作；确认完成后再离开。")}</Notice></div>}{children}
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root></PanelLockContext.Provider>;
}

function recordSettlementCurrency(row: Entity): string | undefined {
  const snapshot = row.values.monetary_snapshot;
  return snapshot && typeof snapshot === "object" && !Array.isArray(snapshot) && typeof snapshot.currency === "string" ? snapshot.currency : undefined;
}

export function ResourceValue({ value, kind = "text", field, settlementCurrency }: { value?: JSONValue; kind?: string; field?: string; settlementCurrency?: string }) {
  const { t, locale } = useI18n();
  const monetary = useMonetary();
  if (field === "unit" && value === "MONEY") return <span>{monetary.currency}</span>;
  if (value === null || value === undefined) return <span className="text-ink-mute">—</span>;
  if (field === "gift_tiers") return <PurchaseTierSummary value={value} currency={settlementCurrency} />;
  if (field === "record_kind" && typeof value === "string") return <span>{recordKindLabel(value, locale)}</span>;
  if (kind === "link" && typeof value === "string" && (value.startsWith("/manage/") || value.startsWith("/api/v1/operators/"))) return <a className="text-brand-emphasis underline" href={value}>{field === "contract_url" ? t("查看私有合同") : value.includes("download") || value.endsWith("/content") ? t("下载私有文件") : t("进入业务办理")}
  </a>;
  if (kind === "details" || typeof value === "object") return <details>
    <summary className="cursor-pointer text-brand-emphasis">{t("查看明细与证据")}</summary>
    <div className="mt-3">
      <ReadFacts value={value} />
    </div>
  </details>;
  if (kind === "money" || kind === "atg") return <Money value={String(value)} unit={kind === "money" ? settlementCurrency || "MONEY" : "ATG"} />;
  if (kind === "datetime" || typeof value === "string" && isFactDate(field, value)) return <When value={String(value)} />;
  if (kind === "status") return <Status value={String(value)} />;
  if (kind === "boolean") return <span className={value ? "text-success" : "text-ink-mute"}>{value ? t("已开启") : t("未开启")}
  </span>;
  return <span className="whitespace-pre-wrap break-words">{resourceValueText(value, field, locale)}
  </span>;
}

export function ReadFacts({ value, field }: { value: JSONValue; field?: string }) {
  const { t, locale } = useI18n();
  const monetary = useMonetary();
  if (field === "unit" && value === "MONEY") return <span>{monetary.currency}</span>;
  field = field ? normalizeFactKey(field) : undefined;
  if (value !== null && typeof value === "object") return <dl className="space-y-3">{Object.entries(value).map(([key, item]) => <div key={key} className="min-w-0 border-l border-hairline pl-3">
    <dt className="text-xs text-ink-mute">{factLabel(key, locale)}
    </dt>
    <dd className="mt-1 break-words text-xs leading-6">
      <ReadFacts value={item} field={key} />
    </dd>
  </div>)}
  </dl>;
  if (typeof value === "string" && isFactDate(field, value)) return <When value={value} />;
  return <span className="whitespace-pre-wrap">{typeof value === "boolean" ? value ? t("是") : t("否") : value === null ? t("未记录") : resourceValueText(value, field, locale)}
  </span>;
}

export function recordTitle(row: Entity, resource: string, locale: Locale = "zh") {
  if (row.presentation?.title) {
    const title = row.presentation.title, prefix = row.presentation.title_kind;
    return prefix && title.startsWith(prefix) ? translate(locale,prefix) + title.slice(prefix.length) : title;
  }
  for (const key of ["name", "display_name", "title", "filename", "hostname", "invoice_reference", "provider_receipt_id", "model", "provider", "kind", "record_kind", "purpose"]) {
    const value = row.values[key];
    if (typeof value === "string" && value) return key === "record_kind" ? recordKindLabel(value, locale) : ["kind", "purpose"].includes(key) ? resourceValueText(value, key, locale) : value;
  }
  return translate(locale, resourceLabel(resource));
}
function recordKindLabel(kind: string, locale: Locale = "zh") { return translate(locale, ({ case: "退款事项", leg: "退款分项", actual_refund_payment: "实际退款付款", exit: "采购退出事项", refund_leg: "退出退款分项", buyer_refund_leg: "采购方退款分项", quote: "采购报价", order: "采购订单", authorization: "自动补货授权", replenishment_attempt: "自动补货执行", credential: "供应商密钥", supplier_model: "供应商模型" } as Record<string, string>)[kind] || kind); }
const factLabels: Record<string, string> = { customer: "客户", customer_id: "客户编号", order_id: "订单编号", receipt_id: "收款编号", provider: "收款通道", currency: "实际币种", original_amount: "原币金额", amount_money: "金额 结算币种", allocated_money: "已分配 结算币种", gross_money: "收款总额（扣手续费前） 结算币种", fee_money: "手续费 结算币种", fee_status: "手续费核实状态", due_money: "应付 结算币种", external_allocated_money: "外部款项已分配 结算币种", internal_money: "结算币种 余额支付", commission_money: "佣金支付", paid_atg: "购买权益 ATG", gift_atg: "赠送权益 ATG", package_gift_atg: "套餐赠送 ATG", merchant_gift_atg: "经营方赠送 ATG", kol_gift_atg: "KOL 赠送 ATG", balance: "余额", held: "占用", purpose: "资产目的", unit: "单位", created_at: "创建时间", paid_at: "真实付款时间", completed_at: "交付时间", expires_at: "到期时间", exception_reason: "异常原因", evidence: "核实依据", verification_evidence: "核实说明或凭证", settlement_amount: "已核实结算金额", recipient_reference: "收款方资料", payer_reference: "付款方资料", status: "业务状态", state: "办理状态", configured: "已配置", customer_money_topup: "客户 结算币种 充值", package_purchase: "套餐购买", package_renewal: "套餐续费", package: "套餐订单", topup: "余额充值", renewal: "套餐续费" };

const editableOperations: Record<string, { name: string; label: string }> = { plans: { name: "publish", label: "编辑套餐" }, marketing: { name: "publish", label: "修订活动" }, catalog: { name: "save", label: "编辑模型" }, routes: { name: "save", label: "编辑路由" }, "payment-methods": { name: "configure", label: "调整配置" } };
const currentRecordOperations: Record<string, string[]> = { mail: ["configure"], commissions: ["policy"], branding: ["theme"], "channel-themes": ["theme"] };

// Only system enum fields are translated; business names and evidence stay verbatim.
export function resourceValueText(value: JSONValue | undefined, field: string | undefined, locale: Locale): string {
  if (typeof value === "boolean") return translate(locale, value ? "是" : "否");
  if (typeof value !== "string") return String(value ?? "—");
  if (field === "record_kind") return recordKindLabel(value, locale);
  if (field && ["metric_name", "direction_name", "api_entry_note", "reserved_note", "payment_hold"].includes(field)) return translate(locale,value);
  if (field === "action") return translate(locale, ({registration:"注册",new_purchase:"新购",renewal:"续费",model_usage:"模型调用",activity:"营销活动",withdrawal:"佣金提现",wallet_refund:"余额退款",customer_login:"登录",all_new_business:"所有新增业务"} as Record<string,string>)[value] || value);
  if (field && enumLabelsByField[field]?.[value]) return translate(locale, enumLabelsByField[field][value]);
  if (field && ["status", "state", "fee_status"].includes(field)) return translate(locale, statusLabels[value] || value);
  if (field === "purpose" && value.startsWith("customer:atg:")) return `${translate(locale, "客户 ATG 权益")} · ${value.slice("customer:atg:".length)}`;
  if (field && ["status", "state", "fee_status", "kind", "purpose", "method", "mode", "metric", "type", "capability"].includes(field)) return translate(locale, systemValueLabels[value] || factLabels[value] || value);
  return value;
}
function normalizeFactKey(key: string): string {
  return key.replace(/([A-Z]+)([A-Z][a-z])/g, "$1_$2").replace(/([a-z0-9])([A-Z])/g, "$1_$2").toLowerCase();
}
export function factLabel(key: string, locale: Locale): string {
  const normalized = normalizeFactKey(key);
  if (normalized.startsWith("lot_refund_money:")) return `${translate(locale, "采购批次退款 结算币种")} · ${key.slice(key.indexOf(":")+1)}`;
  return translate(locale, factLabels[normalized] || savedFactLabels[normalized] || key);
}

const savedFactLabels: Record<string, string> = {
 "used_uses": "已用名额", "reserved_uses": "待付款占用", "total_use_limit": "活动总名额", "combined_campaign_names": "已开放组合的 KOL 活动", "direct_percent": "直接邀请佣金 %", "indirect_percent": "间接邀请佣金 %", "concession_percent": "让利百分点",
  "name": "名称",
  "kind": "业务类型",
  "primary_color": "独立主色",
  "background_color": "独立背景",
  "logo_url": "独立Logo",
  "support_url": "支持入口",
  "file": "Logo PNG/JPEG/WebP（不超过2MiB）",
  "id": "编号",
  "display_name": "显示名称",
  "identity": "事实身份",
  "capabilities": "验证能力",
  "enabled": "启用",
  "provider_id": "供应商",
  "secret": "供应商密钥",
  "expected_version": "当前版本（首次0）",
  "public_model_id": "公开模型",
  "provider": "协议",
  "model": "上游模型",
  "credential_id": "凭据引用",
  "priority": "优先级",
  "healthy": "健康",
  "routing.max_concurrent": "最大并发请求数",
  "routing.failure_threshold": "连续失败次数",
  "routing.unknown_usage_threshold": "用量无法核实次数",
  "routing.recovery_successes": "恢复所需成功次数",
  "routing.cooldown_seconds": "故障暂停时长（秒）",
  "resource_id": "模型/资源",
  "effective_at": "计划生效",
  "announced_at": "实际通知",
  "applied_at": "实际应用",
  "error_code": "待复核原因",
  "rules_reference": "对客户公开的通知条款依据",
  "prices": "精确组件价格",
  "status": "状态",
  "procurement_available": "可用采购余额",
  "required": "已承诺服务所需覆盖 ATG",
  "pending": "待交付所需覆盖 ATG",
  "oversell": "销售超卖额度 ATG",
  "space": "剩余销售承载空间",
  "warning_buffer": "预警缓冲",
  "coverage_reason": "覆盖核实说明",
  "management_scope": "办理权限与履约要求",
  "allowance": "销售超卖额度 ATG",
  "warning": "预警缓冲 ATG",
  "receipt_id": "原收款来源",
  "provider_case_id": "外部争议编号",
  "amount_money": "实际损失",
  "recovered_money": "已核实追回",
  "reason": "原因",
  "created_at": "登记时间",
  "evidence": "核实依据",
  "classification": "事实归类",
  "operator_name": "品牌经营方",
  "hostname": "域名",
  "tls_ready": "有效TLS",
  "verification_token": "TXT验证值",
  "provider_status": "域名供应商状态",
  "tls_status": "证书状态",
  "provider_instructions": "实际DNS步骤",
  "failure_reason": "失败原因",
  "checked_at": "检查时间",
  "target_operator_id": "目标品牌经营方",
  "filename": "原文件名",
  "content_type": "格式",
  "created_by": "创建人编号",
  "download_url": "授权下载",
  "record_kind": "记录类型",
  "buyer_id": "采购方编号",
  "supplier_id": "供货方编号",
  "contract_money": "合同退款责任",
  "book_cost_money": "退出账面成本",
  "cost_difference_money": "合同与成本差额",
  "refunded_money": "已核实退回",
  "action": "办理类型",
  "mode": "办理方式",
  "quantity": "采购库存 ATG",
  "approval_id": "供货失约批准编号（失约退出必填）",
  "proof.receipt_id": "原收款ID",
  "proof.external_reference": "退款流水或收据号（可选）",
  "proof.money.currency": "本次付款币种",
  "proof.money.amount": "本次实际付款金额",
  "proof.executed_at": "实际退款时间",
  "proof.evidence_reference": "付款核实说明",
  "proof.recipient_reference": "原已核实付款人账户",
  "money.currency": "本次付款币种",
  "money.amount": "本次实际执行金额",
  "actual_money": "实际价值 结算币种",
  "settled_money": "已结清 结算币种",
  "remaining_money": "剩余责任 结算币种",
  "excess_money": "多付金额 结算币种",
  "network": "准确网络",
  "asset": "准确资产",
  "token_contract": "代币合约",
  "address": "明确授权地址",
  "authorization_id": "独立核实授权",
  "route_version": "剩余款路线版本",
  "route_history": "已封存历史",
  "fx_execution_reference": "本次执行凭证/原币种结算依据",
  "package_id": "现有套餐编号（新建留空）",
  "price_money": "成交价格 结算币种",
  "paid_atg": "购买权益 ATG",
  "gift_atg": "套餐赠送 ATG",
  "period": "有效期",
  "timezone": "经营时区",
  "terms_reference": "对客户公示条款编号",
  "new_purchase_enabled": "允许新购",
  "renewal_enabled": "允许续费",
  "legal_name": "法定名称",
  "brand_name": "OEM品牌名称",
  "api_base_url": "OEM API入口",
  "threshold_money": "终身资格累计净消费门槛 结算币种",
  "direct_rate": "直接推荐分佣比例（0到1）",
  "indirect_rate": "间接推荐分佣比例（0到1）",
  "confirmation_seconds": "消费确认等待天数",
  "confirmation_days": "消费确认等待天数",
  "invitee_atg": "受邀人注册赠送 ATG",
  "inviter_atg": "邀请人注册赠送 ATG",
  "reward_validity_seconds": "注册奖励有效天数",
  "reward_validity_days": "注册奖励有效天数",
  "kol_cross_activity_limit": "KOL跨活动限制次数",
  "campaign_id": "现有活动编号（新建留空）",
  "eligibility": "参与对象",
  "final_price_money": "最终成交价 结算币种（无折扣留空）",
  "concession_rate": "本人佣金让利比例（0到1）",
  "starts_at": "开始时间",
  "ends_at": "结束时间",
  "per_customer_limit": "每位客户限次",
  "feature": "营业项目",
  "evidence_reference": "检查及授权依据",
  "text_color": "文字色（留空继承）",
  "recipient_id": "本经营范围收件人编号",
  "category": "通知类别",
  "severity": "级别",
  "title": "标题",
  "body": "内容",
  "reference": "业务关联依据",
  "owner_id": "限定主体编号（留空当前范围）",
  "from": "起始时间",
  "to": "截至时间",
  "metric": "真实指标",
  "critical": "严重阈值",
  "direction": "触发方向",
  "recipients": "通知收件人ID（逗号分隔）",
  "escalation_recipients": "升级收件人ID（逗号分隔）",
  "reminder_seconds": "持续风险提醒间隔秒数",
  "escalate_seconds": "升级等待秒数",
  "provider_receipt_id": "外部唯一收款流水",
  "gross_money": "收款总额（扣手续费前） 结算币种",
  "fee_status": "手续费证据",
  "fee_money": "已核实手续费结算币种",
  "net_money": "净额 结算币种",
  "currency": "实际入账币种",
  "original_amount": "原币金额",
  "recipient_reference": "收款方资料（可选）",
  "payer_reference": "付款方资料（可选）",
  "settlement_amount": "已核实结算金额",
  "verification_evidence": "收款说明或凭证（可选）",
  "paid_at": "真实付款时刻",
  "observed_at": "核实时刻",
  "settled_at": "费用结算时刻（已确认必填）",
  "driver": "真实通道协议",
  "asset_network_version_id": "平台已发布的准确资产与网络",
  "payment_reference": "真实唯一付款流水",
  "paid_money": "原成交 结算币种",
  "minimum_money": "最低提现 结算币种",
  "flat_fee_money": "固定手续费 结算币种",
  "fee_rate": "手续费比例（0到1）",
  "requires_dual_approval": "必须双人审批付款",
  "method_reference": "对客户公示付款方式规则",
  "issuer_id": "原始开票经营方",
  "lines.0.receipt_id": "原始收款编号",
  "lines.0.amount_money": "本次申请 结算币种",
  "details.title": "发票抬头",
  "details.tax_id": "纳税识别号",
  "details.email": "接收邮箱",
  "details.address": "发票地址",
  "customer_id": "客户编号",
  "order_id": "原成交订单",
  "second_review_above": "需独立复核的结算币种阈值（留空关闭）",
  "amount": "实际币种金额",
  "occurred_at": "实际发生时间",
  "operator_id": "经营方编号",
  "invoice_reference": "发票凭证编号",
  "host": "SMTP服务器",
  "port": "端口",
  "security": "传输安全",
  "password_set": "密码已配置",
  "available": "通过配置预检",
  "source": "配置来源",
  "username": "SMTP认证用户名",
  "password": "新密码（留空保留）",
  "amount_atg": "独立赠送ATG",
  "expires_at": "到期（空为不限期）",
  "grant_id": "独立权益来源",
  "state": "状态",
  "value": "精确值",
  "notification_id": "通知",
  "attempts": "尝试次数",
  "next_attempt_at": "下次尝试",
  "domain": "领域",
  "unit": "单位",
  "expected": "应有",
  "actual": "实际",
  "adjustment": "真实冲正/修复记录",
  "row_count": "记录数",
  "party_kind": "办理对象",
  "stage": "允许开票阶段",
  "maximum_ratio": "最高原收款比例（0到1）",
  "token_decimals": "小数位",
  "minimum_confirmations": "最少确认数",
  "asset_id": "已有组合（首次留空）",
  "allocated_money": "已分配结算币种",
  "exit_terms": "原退出条款",
  "exit_terms.version": "退出条款版本",
  "exit_terms.voluntary_refund": "允许自愿退出退款",
  "exit_terms.method": "合同退款算法",
  "exit_terms.terms_reference": "退出条款编号",
  "valid_until": "报价截止时间",
  "quote_id": "固定成交报价编号",
  "trigger_available_atg": "采购可用库存低于ATG时触发",
  "period_limit_money": "周期最高结算币种",
  "merchant_id": "真实供货收款商户配置编号",
  "payment.payment_method": "Stripe已授权pm_编号",
  "payment.customer_ref": "Stripe已授权cus_编号",
  "consent_reference": "独立自动付款授权凭证",
  "transaction_hash": "实际交易Hash（仅查找线索）",
  "log_index": "目标Transfer事件索引（十六进制）",
  "expected_execution_key": "当前原执行编号",
  "result": "执行方已核实终态",
  "external_reference": "外部终态核实流水",
  "no_further_payment_reference": "执行方不会继续付款的独立证明",
  "verified_at": "真实核实时刻",
  "proposal_id": "本轮买方提案",
  "evm.transaction_hash": "EVM实际交易Hash（链上必填）",
  "evm.log_index": "EVM真实日志索引（0x）",
  "external.external_reference": "银行/NOW实际付款流水",
  "external.money.currency": "银行/NOW实际准确币种",
  "external.money.amount": "银行/NOW实际原币金额",
  "external.executed_at": "银行/NOW真实执行时刻",
  "external.evidence_reference": "银行/NOW核实凭证",
  "payer_authorization_reference": "原付款人明确授权依据",
  "money": "已实际收到原币",
  "actual_paid_at": "原事实实际付款时间",
  "payment_time_evidence": "独立付款时点证明（补时间时必填）",
  "destination.network": "准确退款网络（银行填bank，EVM填十进制Chain ID）",
  "destination.asset": "原收款准确币种或资产代码",
  "destination.token_contract": "代币合约（银行留空）",
  "destination.address": "本人明确指定接收地址或银行账户凭证",
  "history": "已封存授权历史",
  "authorization_reference": "本人明确授权与地址来源说明",
  "responsible_execution": "本人明确选择责任方付款",
  "case_id": "原申请",
  "decision": "审核决定",
  "route_exception_reference": "同经营方原路受限的替代依据（跨经营方接管可留空）",
  "leg_id": "原退款责任",
  "actual_payment_money": "本次实际支付结算币种",
  "configuration": "配置 JSON",
  "due_money": "应付",
  "external_allocated_money": "已分配",
  "external_method_id": "收款方式",
  "total": "总额",
  "consumed": "已消费",
  "expired": "已过期",
  "cancelled": "已取消",
  "email": "邮箱",
  "brand_id": "品牌身份范围",
  "current_password": "当前密码",
  "new_email": "新邮箱",
  "waiver_evidence": "本人明确放弃购买与赠送ATG的确认",
  "registered_at": "注册时间",
  "missing_fields": "待配置材料",
  "fields_set": "已设置字段",
  "Currency": "币种",
  "Source": "来源依据",
  "ExpiresAt": "有效截止",
  "purpose": "账户目的",
  "balance": "余额",
  "held": "占用",
  "used_count": "使用次数",
  "used_gift_atg": "累计赠送",
  "release_blocked": "可疑审查：暂缓解冻与新使用",
  "merchant_campaign_id": "经营方活动",
  "kol_campaign_id": "KOL活动",
  "registration": "注册",
  "new_purchase": "新购",
  "renewal": "续费",
  "model_usage": "调用",
  "activity": "营销",
  "settlement": "结算",
  "staff_id": "人员编号",
  "role": "角色",
  "scope_kind": "范围",
  "scope_id": "主体",
  "explanation": "对客户说明",
  "review_due_at": "复核到期",
  "subject_id": "关联主体",
  "actor_id": "实际操作者",
  "before": "变更前记录",
  "after": "变更后记录",
  "data": "原始业务事实",
  "requested_by": "申请人",
  "workspace": "进入原业务复核",
  "estimate_atg": "预估占用",
  "actual_atg": "实际用量计价",
  "charged_atg": "实扣ATG",
  "supplier_reference_costs": "供应商参考成本",
  "supplier_reported_costs": "供应商报告成本",
  "unverified_attempts": "成本证据状态",
  "accepted_at": "受理时间",
  "rule_id": "风险规则",
  "opened_at": "本次风险开始",
  "last_notified_at": "最近通知",
  "policy": "实际策略版本",
  "health": "候选路由健康与占用",
  "strategy": "选择策略",
  "grants": "自定义权限",
  "queue": "队列",
  "business_key": "业务操作编号",
  "max_attempts": "重试上限",
  "last_error": "上次错误",
  "available_at": "可执行时间",
  "additional_attempts": "追加尝试次数1–10",
  "legal_hold_until": "保护截至",
  "until": "保护截止时间",
  "configuration.minimum_money": "最低提现 结算币种",
  "configuration.method_reference": "对客户公示付款方式规则",
  "configuration.flat_fee_money": "固定手续费 结算币种",
  "configuration.fee_rate": "手续费比例（0到1）",
  "configuration.requires_dual_approval": "必须双人审批付款",
  "configuration.expected_version": "当前规则版本",
  "verified_unpaid": "已核实未付",
  "model_id": "公开模型",
  "operation": "调用类型",
  "net_charged_atg": "冲正后净扣费",
  "hop": "邀请层级",
  "reversed_money": "已冲正",
  "confirm_at": "确认时间",
  "manage_url": "办理",
  "order_allocated_money": "分配至此单",
  "account_id": "账户编号",
  "channel_id": "渠道编号",
  "brand_operator_id": "品牌经营方编号",
  "cost": "采购账面成本 结算币种",
  "has_upstream": "存在上级供货方",
  "version": "版本",
  "service_atg": "承接服务 ATG",
  "service_money": "承接服务作价",
  "cash_money": "承接现金与佣金责任",
  "offset_atg": "采购库存抵扣 ATG",
  "offset_money": "采购库存抵扣 结算币种",
  "residual_refund_money": "剩余合同可退采购款 结算币种",
  "settlement_due_money": "合同结清额 结算币种",
  "receipt_reference": "收款凭证引用",
  "remaining": "剩余采购库存 ATG",
  "paid_consumed": "已消费购买库存 ATG",
  "gift_consumed": "已消费赠送库存 ATG",
  "paid_removed": "已移除购买库存 ATG",
  "gift_removed": "已移除赠送库存 ATG",
  "fingerprint": "记录指纹",
  "delivered_at": "交付时间",
  "lot_refund_money": "采购批次退款 结算币种",
  "rounding_evidence": "舍入依据",
  "lots": "采购批次",
  "coverage": "覆盖与责任计算",
  "procurement": "采购库存原始头寸",
  "customers": "完整客户范围",
  "assets": "资产明细",
  "notes": "备注",
  "type": "类型",
  "method": "合同作价方式",
  "subject": "主题",
  "snapshot": "原始快照",
  "source_kind": "资金来源类型",
  "source_id": "来源编号",
  "updated_by": "更新人编号",
  "updated_at": "更新时间",
  "deleted_at": "删除时间",
  "contract_reference": "双方接管合同编号"
};

const enumLabelsByField: Record<string, Record<string, string>> = {
  "classification": {
    "invalid_payment": "付款无效，按来源纠正",
    "merchant_collection_risk": "经营收款风险，保留客户权益",
    "reversed": "外部已撤销且全部追回"
  },
  "mode": {
    "voluntary_exit": "按合同自愿退出",
    "supply_failure": "已有供货失约批准",
    "offline": "仅建立待付订单",
    "automatic": "已获授权Stripe自动扣款"
  },
  "period": {
    "lifetime": "不限期",
    "month": "月",
    "quarter": "季",
    "year": "年"
  },
  "kind": {
    "supply": "供货采购",
    "oem": "独立OEM品牌",
    "channel": "同品牌渠道",
    "ledger": "账本",
    "requests": "请求",
    "procurement": "采购"
  },
  "eligibility": {
    "any": "符合当前活动资格",
    "first_order": "首购客户",
    "existing": "已有成交客户"
  },
  "feature": {
    "registration": "注册",
    "new_purchase": "新购",
    "renewal": "续费",
    "model_usage": "模型调用",
    "activity": "营销活动",
    "settlement": "结算"
  },
  "category": {
    "mandatory": "必要业务通知",
    "usage": "用量提醒",
    "marketing": "已获授权营销"
  },
  "severity": {
    "info": "一般",
    "warning": "预警",
    "critical": "紧急"
  },
  "metric": {
    "sales_space_atg": "剩余销售空间ATG",
    "procurement_available_atg": "采购可用ATG",
    "oversell_used_atg": "已用超卖ATG",
    "pending_usage": "待结请求数",
    "unverified_costs": "未知成本数"
  },
  "direction": {
    "below": "低于阈值",
    "above": "高于阈值"
  },
  "fee_status": {
    "pending": "尚待核实",
    "confirmed": "已核实"
  },
  "driver": {
    "stripe": "Stripe",
    "alipay": "支付宝",
    "wechat": "微信支付",
    "nowpayments": "NOWPayments",
    "evm_wallet": "自有EVM钱包",
    "manual": "线下人工核实"
  },
  "reason": {
    "transaction_error": "交易错误",
    "unable_to_fulfill": "无法履行服务",
    "compensation": "独立服务补偿",
    "targeted_gift": "受权定向赠送",
    "failed": "已失败且不会再付",
    "terminated": "已终止且不会再付",
    "settled_partial": "实际少付且旧指令已结束"
  },
  "security": {
    "starttls": "STARTTLS",
    "tls": "TLS",
    "plain": "明文（仅隔离测试）"
  },
  "stage": {
    "receipt": "原始收款",
    "top_up": "充值完成",
    "purchase": "完成购买",
    "consumption": "实际消费"
  },
  "state": {
    "issued": "已开具",
    "rejected": "不予办理",
    "voided": "已作废"
  },
  "exit_terms.method": {
    "remaining_paid_proportional": "剩余购买库存原价比例",
    "non_refundable": "合同不退款"
  },
  "result": {
    "failed": "实际失败",
    "cancelled": "执行方已取消"
  },
  "decision": {
    "approve": "批准按原责任办理",
    "reject": "拒绝并保留审核原因"
  },
  "status": {
    "active": "启用",
    "suspended": "停用",
    "terminated": "永久终止本人后续分佣，不可恢复"
  },
  "role": {
    "owner": "负责人",
    "operations": "运营",
    "finance": "财务",
    "support": "客服",
    "readonly": "只读",
    "custom": "明确自定义"
  },
  "scope_kind": {
    "operator": "本经营方",
    "brand": "本品牌（仅品牌负责人）",
    "customer": "当前服务归属客户"
  },
  "action": {
    "registration": "注册",
    "new_purchase": "新购",
    "renewal": "续费",
    "model_usage": "模型调用",
    "activity": "营销活动",
    "withdrawal": "佣金提现",
    "wallet_refund": "余额退款",
    "customer_login": "登录",
    "all_new_business": "所有新增业务"
  },
  "strategy": {
    "priority": "固定优先级",
    "lowest_latency": "已验证最低延迟",
    "lowest_cost": "已验证最低成本"
  }
};

const systemValueLabels: Record<string, string> = {
  "customer:money": "客户 结算币种 现金余额",
  "customer:atg": "客户 ATG 权益",
  "commission:available": "可用佣金",
  "commission:frozen": "冻结佣金",
  "control:takeover": "接管权益对账",
  "control:takeover_settlement": "接管结清对账",
  "control:external_cash": "外部现金往来",
  "control:merchant_fees": "经营方手续费",
  "takeover_cash_settlement": "接管实际资金结清",
  "takeover_counterparty_settlement": "接管对手方结清",
  "supply_coverage": "按供货覆盖计算",
  "agreed_atg_value": "按约定ATG价格",
  "sales_space_atg": "剩余销售承载空间",
  "procurement_available_atg": "可用采购余额",
  "oversell_used_atg": "已使用超卖空间",
  "pending_receipt_facts": "实际款待核实与入账",
  "awaiting_settlement_receipts": "收款待核实结算金额",
  "pending_usage": "用量待核实请求",
  "unverified_costs": "供应成本待核实"
};

function isFactDate(field: string | undefined, value: string): boolean {
  const key = field ? normalizeFactKey(field) : "";
  return /(?:_at|_from|_until)$/.test(key) || ["from", "to", "date", "timestamp"].includes(key) ? /^\d{4}-\d{2}-\d{2}T/.test(value) : false;
}
