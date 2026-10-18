"use client";

import { createContext, useCallback, useContext, useEffect, useId, useState, type ReactNode } from "react";
import Link from "next/link";
import * as Dialog from "@radix-ui/react-dialog";
import { useQuery } from "@tanstack/react-query";
import { X, ArrowUpRight, Search, SlidersHorizontal } from "lucide-react";
import { request } from "@/lib/api-client";
import { ResourcePageSchema, type Entity, type Operation, type JSONValue, type ResourcePage } from "@/lib/contracts";
import { canReadResource, resourceLabel } from "@/lib/navigation";
import { useNavigationBlock } from "@/lib/navigation-guard";
import { useProduct } from "./context";
import { InvoiceFiles } from "./invoice-files";
import { OperationForm } from "./operation-form";
import { Empty, ErrorPanel, Heading, Loading, Money, Notice, Stat, Status, Table, When } from "./shared";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";

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
};

export function ResourceScreen({ operatorID, resource, customer = false, embedded = false, layout = "table", onInspect, record }: ResourceOptions) {
  const { session } = useProduct();
  const [cursor, setCursor] = useState(""), [cursorHistory, setCursorHistory] = useState<string[]>([]);
  const [query, setQuery] = useState(""), [search, setSearch] = useState(""), [view, setView] = useState("");
  const searchDescription = useId();
  const scope = operatorID ? `/api/v1/operators/${encodeURIComponent(operatorID)}` : "/api/v1/me";
  const resourceName = resource.split("/")[0];
  // Only advertise a scope-wide search where the corresponding endpoint
  // actually applies q before pagination. Other lists keep an honest local
  // filter and retain their original cursor when the filter changes.
  const serverSearch = resourceName === "invoices" || (!customer && ["customers", "orders", "assets", "ledger", "gifts"].includes(resourceName));
  const parameters = new URLSearchParams();
  if (cursor) parameters.set("cursor", cursor);
  if (serverSearch && search) parameters.set("q", search);
  if (view) parameters.set("view", view);
  if (record) parameters.set("record", record);
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const canRead = customer ? session?.kind === "customer" : canReadResource(workspace, resource.split("/")[0]);
  const data = useQuery({ queryKey: ["resource", scope, resource, cursor, serverSearch ? search : "", view, record || ""], queryFn: ({ signal }) => request(`${scope}/${resource}${parameters.size ? `?${parameters}` : ""}`, ResourcePageSchema, { signal }), enabled: canRead });
  if (!canRead) return <Empty title="没有查看权限">此页面需要当前经营方明确授予相应权限。</Empty>;
  if (data.isPending) return <Loading />;
  if (data.isError) return <ErrorPanel error={data.error} retry={() => void data.refetch()} />;
  const page = data.data;
  const filter = search.toLocaleLowerCase();
  const visiblePage = !serverSearch && filter ? { ...page, items: page.items.filter(item => JSON.stringify([item.id, item.status, ...page.columns.flatMap(column => {
    const value = item.values[column.key];
    return column.key === "record_kind" && typeof value === "string" ? [value, recordKindLabel(value)] : [value];
  })]).toLocaleLowerCase().includes(filter)) } : page;
  const reset = () => { setCursor(""); setCursorHistory([]); };
  return <>
    {record && <div className="mb-5">
      <Notice>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span>正在办理关联事项 <span className="break-all font-mono text-xs">{record}
          </span>
          </span>
          <Link className="text-brand-emphasis underline" href={operatorID ? `/manage/${encodeURIComponent(operatorID)}/${resource}` : `/workspace/${resource}`}>返回全部记录</Link>
        </div>
      </Notice>
    </div>}
    {page.views && <div aria-label="业务视图" className="mb-5 flex flex-wrap gap-2">{page.views.map(item => <Button key={item.value} variant={(view || page.views?.[0]?.value) === item.value ? "default" : "outline"} onClick={() => { setView(item.value); reset(); }}>{item.label}
    </Button>)}
    </div>}
    <ResourceCollection page={visiblePage} operatorID={operatorID} resource={resource} customer={customer} embedded={embedded} layout={layout} onInspect={onInspect} toolbar={<form className="w-full max-w-xl" onSubmit={event => { event.preventDefault(); if (serverSearch) reset(); setSearch(query.trim()); }}>
      <div className="flex gap-2"><div className="relative min-w-0 flex-1">
        <Search size={15} aria-hidden className="absolute left-3 top-3.5 text-ink-mute" />
        <Input aria-label="搜索记录" aria-describedby={searchDescription} placeholder={resourceName === "customers" ? "搜索客户名称或邮箱" : resourceName === "invoices" ? "搜索发票编号、开票凭证或原始收款编号" : serverSearch ? "搜索业务编号、名称或关联记录" : "筛选本页编号或已显示字段"} value={query} onChange={event => setQuery(event.target.value)} className="pl-9" />
      </div>
      <Button variant="outline" type="submit">{serverSearch ? "查询" : "筛选本页"}</Button>{search && <Button variant="ghost" type="button" onClick={() => { setQuery(""); setSearch(""); if (serverSearch) reset(); }}>清除</Button>}</div>
      <p id={searchDescription} className="mt-2 text-xs leading-6 text-ink-mute">{serverSearch ? "在当前权限范围内检索记录，结果按当前列表规则分页。" : "仅筛选当前已载入页；其他页尚未检索，可翻页继续筛选。"}</p>
    </form>} />
    <div className="mt-5 flex items-center justify-between gap-3">
      <p className="text-xs text-ink-mute">{!serverSearch && search ? `本页匹配 ${visiblePage.items.length} / ${page.items.length} 条` : `当前页 ${page.items.length} 条`}{page.next_cursor ? " · 还有后续记录" : ""}
      </p>
      <div className="flex gap-2">{cursorHistory.length > 0 && <Button variant="outline" onClick={() => { const previous = [...cursorHistory]; setCursor(previous.pop() || ""); setCursorHistory(previous); }}>上一页</Button>}{page.next_cursor && <Button variant="outline" onClick={() => { setCursorHistory(previous => [...previous, cursor]); setCursor(page.next_cursor || ""); }}>下一页</Button>}
      </div>
    </div>
  </>;
}

export function ResourceCollection({ page, operatorID, resource, customer = false, embedded = false, layout = "table", onInspect, toolbar, initialValues, actionsOnly = false }: ResourceOptions & { page: ResourcePage; toolbar?: ReactNode; initialValues?: Record<string, JSONValue>; actionsOnly?: boolean }) {
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const [selection, setSelection] = useState<{ operation: Operation; row?: Entity }>();
  const [blocked, setBlocked] = useState(false), [invoiceID, setInvoiceID] = useState(""), [facts, setFacts] = useState<Entity>();
  const [recordKind, setRecordKind] = useState("");
  const resourceName = resource.split("/")[0];
  const canOperate = (operation: Operation) => !operation.permission || !!workspace?.permissions.includes("*") || !!workspace?.permissions.includes(operation.permission);
  const operations = page.operations.filter(canOperate);
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
  const inspectLabel = resourceName === "customers" ? "客户工作台" : resourceName === "orders" ? "查看并办理" : "查看详情";
  const kinds = [...new Set(page.items.map(row => row.values.record_kind).filter((value): value is string => typeof value === "string"))];
  const rows = recordKind ? page.items.filter(row => row.values.record_kind === recordKind) : page.items;
  const actions = (row: Entity) => <div className="flex flex-wrap gap-2">
    <Button type="button" variant="outline" size="sm" onClick={() => inspect(row)}>{inspectLabel}
      <ArrowUpRight size={13} aria-hidden className="ml-1" />
    </Button>{editOperation && (!row.allowed_actions || row.allowed_actions.includes(editOperation.name)) && <Button type="button" variant="outline" size="sm" onClick={() => selectOperation(editOperation, row)}>{editable.label}
    </Button>}{rowOperations(row).map(operation => <Button key={operation.name} type="button" variant="outline" size="sm" onClick={() => selectOperation(operation, row)}>{operation.label}
    </Button>)}{["invoices", "procurement-invoices"].includes(resourceName) && <Button type="button" variant="outline" size="sm" onClick={() => setInvoiceID(row.id)}>文件与下载</Button>}
  </div>;
  return <>
    {!embedded && <Heading title={page.title === resourceName ? resourceLabel(resourceName) : page.title} description={page.description} />}
    {page.summary && <div className="mb-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{page.summary.map((item, index) => <Stat key={index} {...item} />)}
    </div>}
    {page.availability?.filter(item => !item.enabled).map(item => <div key={item.capability} className="mb-4">
      <Notice>{item.reason || "相应业务暂未开放"}{item.next_action && <p>{item.next_action}
      </p>}
      </Notice>
    </div>)}
    {(toolbar || operations.length > 0) && <div className="mb-5 flex flex-wrap items-center justify-between gap-3">{toolbar}
      <div className="flex flex-wrap gap-2">{operations.map((operation, index) => <Button key={operation.name} type="button" variant={index === 0 ? "default" : "outline"} onClick={() => selectOperation(operation)}>{operation.label}
      </Button>)}
      </div>
    </div>}
    {kinds.length > 1 && <div className="mb-5 flex flex-wrap items-center gap-2 text-xs text-ink-mute">
      <SlidersHorizontal size={14} aria-hidden />
      <span>本页类型</span>{["", ...kinds].map(kind => <Button key={kind} size="sm" variant={recordKind === kind ? "default" : "ghost"} onClick={() => setRecordKind(kind)}>{kind ? recordKindLabel(kind) : "全部"}
      </Button>)}
    </div>}
    {actionsOnly ? <div className="flex flex-wrap gap-2">{rows.flatMap(row => rowOperations(row).map(operation => <Button key={`${row.id}:${operation.name}`} type="button" variant="outline" onClick={() => setSelection({ operation, row })}>{operation.label}
    </Button>))}
    </div> : layout === "cards" ? <div className="grid gap-4 xl:grid-cols-2">{rows.map(row => <Card key={row.id} className="min-w-0">
      <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-base font-semibold">{recordTitle(row, resourceName)}
          </h3>
          <p className="mt-1 break-all font-mono text-[11px] text-ink-mute">{row.id}
          </p>
        </div>{row.status && <Status value={row.status} />}
      </div>
      <dl className="mb-5 grid gap-x-5 gap-y-4 sm:grid-cols-2">{page.columns.filter(column => !["id", "status", "name", "display_name"].includes(column.key)).map(column => <div key={column.key} className="min-w-0">
        <dt className="mb-1 text-xs text-ink-mute">{column.label}
        </dt>
        <dd className="min-w-0 text-sm">
          <ResourceValue value={row.values[column.key]} kind={column.kind} field={column.key} />
        </dd>
      </div>)}
      </dl>{actions(row)}
    </Card>)}{!rows.length && <div className="xl:col-span-2">
      <Empty>当前筛选下没有记录。</Empty>
    </div>}
    </div> : <Table rows={rows} getID={row => row.id} empty="当前筛选下没有记录。可查询其他编号或继续办理新事项。" columns={[...page.columns.map(column => ({ label: column.label, cell: (row: Entity) => <ResourceValue value={column.key === "id" ? row.id : column.key === "status" ? row.status || null : row.values[column.key]} kind={column.kind} field={column.key} /> })), ...(!page.columns.some(column => ["status", "state"].includes(column.key)) ? [{ label: "状态", cell: (row: Entity) => row.status ? <Status value={row.status} /> : <span>—</span> }] : []), { label: "办理", cell: actions }]} />}
    {selection && <OperatorPanel title={selection.operation.label} description={selection.row ? `${recordTitle(selection.row, resourceName)} · ${selection.row.id}` : `在当前${customer ? "客户" : "经营"}范围内办理`} open blocked={blocked} close={closeOperation}>
      <OperationForm key={`${selection.operation.name}:${selection.row?.id || "new"}`} operation={selection.operation} row={selection.row} operatorID={operatorID} embedded initialValues={{ ...savedValues, ...initialValues }} lockedFields={Object.keys(initialValues || {})} onBlockedChange={setBlocked} />
    </OperatorPanel>}
    {invoiceID && <InvoiceFiles scope={`${scope}/${resourceName}`} invoiceID={invoiceID} upload={!customer && resourceName === "invoices"} close={() => setInvoiceID("")} />}
    {facts && <OperatorPanel open title={recordTitle(facts, resourceName)} description={`记录 ${facts.id}`} close={() => setFacts(undefined)}>
      <div className="mb-5">{facts.status && <Status value={facts.status} />}
      </div>
      <dl className="grid gap-5 sm:grid-cols-2">{page.columns.filter(column => column.key !== "id").map(column => <div key={column.key} className="min-w-0">
        <dt className="mb-2 text-xs text-ink-mute">{column.label}
        </dt>
        <dd>
          <ResourceValue value={column.key === "status" ? facts.status : facts.values[column.key]} kind={column.kind} field={column.key} />
        </dd>
      </div>)}
      </dl>
      <details className="mt-7 border-t border-hairline pt-5">
        <summary className="cursor-pointer text-sm text-brand-emphasis">完整保存记录与证据</summary>
        <div className="mt-4">
          <ReadFacts value={facts.values} />
        </div>
      </details>
      <div className="mt-6 flex flex-wrap gap-2">{rowOperations(facts).map(operation => <Button key={operation.name} variant="outline" onClick={() => { setFacts(undefined); setSelection({ operation, row: facts }); }}>{operation.label}
      </Button>)}
      </div>
    </OperatorPanel>}
  </>;
}

export function OperatorPanel({ title, description, open, close, blocked = false, children, onBlockedChange }: { title: string; description: string; open: boolean; close: () => void; blocked?: boolean; children: ReactNode; onBlockedChange?: (blocked: boolean) => void }) {
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
      <Dialog.Overlay className="fixed inset-0 z-40 bg-black/35 backdrop-blur-[2px]" />
      <Dialog.Content className="fixed inset-y-0 right-0 z-50 w-full overflow-y-auto border-l border-hairline bg-canvas p-5 shadow-2xl focus:outline-none sm:max-w-4xl sm:p-8" onEscapeKeyDown={event => { if (locked) event.preventDefault(); }} onPointerDownOutside={event => { if (locked) event.preventDefault(); }} onInteractOutside={event => { if (locked) event.preventDefault(); }}>
        <div className="mb-7 flex items-start justify-between gap-5">
          <div className="min-w-0">
            <Dialog.Title className="text-2xl font-semibold tracking-tight">{title}
            </Dialog.Title>
            <Dialog.Description className="mt-2 break-words text-sm leading-6 text-ink-mute">{description}
            </Dialog.Description>
          </div>
          <Button variant="ghost" size="icon" aria-label="关闭办理面板" disabled={locked} onClick={close}>
            <X size={20} />
          </Button>
        </div>{navigationBlocked && <div className="mb-5"><Notice>本次办理尚未取得确定结果，请保持此面板并重试原操作；确认完成后再离开。</Notice></div>}{children}
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root></PanelLockContext.Provider>;
}

export function ResourceValue({ value, kind = "text", field }: { value?: JSONValue; kind?: string; field?: string }) {
  if (value === null || value === undefined) return <span className="text-ink-mute">—</span>;
  if (field === "record_kind" && typeof value === "string") return <span>{recordKindLabel(value)}</span>;
  if (kind === "link" && typeof value === "string" && (value.startsWith("/manage/") || value.startsWith("/api/v1/operators/"))) return <a className="text-brand-emphasis underline" href={value}>{value.includes("download") ? "下载私有文件" : "进入业务办理"}
  </a>;
  if (kind === "details" || typeof value === "object") return <details>
    <summary className="cursor-pointer text-brand-emphasis">查看明细与证据</summary>
    <div className="mt-3">
      <ReadFacts value={value} />
    </div>
  </details>;
  if (kind === "usd" || kind === "atg") return <Money value={String(value)} unit={kind.toUpperCase()} />;
  if (kind === "datetime") return <When value={String(value)} />;
  if (kind === "status") return <Status value={String(value)} />;
  if (kind === "boolean") return <span className={value ? "text-success" : "text-ink-mute"}>{value ? "已开启" : "未开启"}
  </span>;
  return <span className="whitespace-pre-wrap break-words">{String(value)}
  </span>;
}

export function ReadFacts({ value }: { value: JSONValue }) {
  if (value !== null && typeof value === "object") return <dl className="space-y-3">{Object.entries(value).map(([key, item]) => <div key={key} className="min-w-0 border-l border-hairline pl-3">
    <dt className="text-xs text-ink-mute">{factLabels[key] || key}
    </dt>
    <dd className="mt-1 break-words text-xs leading-6">
      <ReadFacts value={item} />
    </dd>
  </div>)}
  </dl>;
  return <span className="whitespace-pre-wrap">{typeof value === "boolean" ? value ? "是" : "否" : String(value ?? "未记录")}
  </span>;
}

export function recordTitle(row: Entity, resource: string) {
  for (const key of ["name", "display_name", "model", "provider", "kind", "record_kind", "purpose"]) {
    const value = row.values[key];
    if (typeof value === "string" && value) return key === "record_kind" ? recordKindLabel(value) : factLabels[value] || value;
  }
  return resourceLabel(resource);
}
function recordKindLabel(kind: string) { return ({ case: "退款事项", leg: "退款分项", actual_refund_payment: "实际退款付款", exit: "采购退出事项", refund_leg: "退出退款分项", buyer_refund_leg: "采购方退款分项", quote: "采购报价", order: "采购订单", authorization: "自动补货授权", replenishment_attempt: "自动补货执行" } as Record<string, string>)[kind] || kind; }
const factLabels: Record<string, string> = { customer: "客户", customer_id: "客户编号", order_id: "订单编号", receipt_id: "收款编号", provider: "收款通道", currency: "实际币种", original_amount: "原币金额", amount_usd: "金额 USD", allocated_usd: "已分配 USD", gross_usd: "实际入账 USD", fee_usd: "手续费 USD", fee_status: "手续费核实状态", due_usd: "应付 USD", external_allocated_usd: "外部款项已分配 USD", internal_usd: "USD 余额支付", commission_usd: "佣金支付", paid_atg: "购买权益 ATG", gift_atg: "赠送权益 ATG", package_gift_atg: "套餐赠送 ATG", merchant_gift_atg: "经营方赠送 ATG", kol_gift_atg: "KOL 赠送 ATG", balance: "余额", held: "占用", purpose: "资产目的", unit: "单位", created_at: "创建时间", paid_at: "真实付款时间", completed_at: "交付时间", expires_at: "到期时间", exception_reason: "异常原因", evidence: "核实依据", verification_evidence: "银行核实凭证", valuation_evidence: "估值依据", recipient_reference: "收款账号证明", payer_reference: "付款账号证明", status: "业务状态", state: "办理状态", configured: "已配置", customer_usd_topup: "客户 USD 充值", package_purchase: "套餐购买", package_renewal: "套餐续费", package: "套餐订单", topup: "余额充值", renewal: "套餐续费" };

const editableOperations: Record<string, { name: string; label: string }> = { plans: { name: "publish", label: "修订套餐" }, marketing: { name: "publish", label: "修订活动" }, providers: { name: "credential", label: "轮换凭据" }, catalog: { name: "save", label: "编辑模型" }, routes: { name: "save", label: "编辑路由" }, "payment-methods": { name: "configure", label: "调整配置" } };
const currentRecordOperations: Record<string, string[]> = { commissions: ["policy"], "business-status": ["set"], branding: ["theme"], "channel-themes": ["theme"] };
