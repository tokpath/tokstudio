"use client";
import { FinancialReportScreen } from "./financial-report";
import { InvoiceFiles } from "./invoice-files";
import { TakeoversScreen } from "./takeovers";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { ResourcePageSchema, type Entity, type Operation, type JSONValue } from "@/lib/contracts";
import { resourceLabel } from "@/lib/navigation";
import { useProduct } from "./context";
import { WorkspaceShell } from "./workspace-shell";
import { OperationForm } from "./operation-form";
import { Empty, ErrorPanel, Heading, Loading, Money, Notice, Stat, Status, Table, When } from "./shared";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function ManagementPage({ operatorID, resource }: { operatorID: string; resource: string }) { return <WorkspaceShell operatorID={operatorID}>{resource==="reports"?<FinancialReportScreen operatorID={operatorID}/>: ["takeovers","takeover-settlements"].includes(resource)?<TakeoversScreen operatorID={operatorID} settlements={resource==="takeover-settlements"}/>:<ResourceScreen operatorID={operatorID} resource={resource} />}</WorkspaceShell>; }
export function ResourceScreen({ operatorID, resource, customer = false }: { operatorID?: string; resource: string; customer?: boolean }) {
  const { session } = useProduct();
 const [invoiceID,setInvoiceID]=useState("");
  const [cursor, setCursor] = useState(""), [cursorHistory, setCursorHistory] = useState<string[]>([]), [query, setQuery] = useState(""), [search, setSearch] = useState(""),[view,setView]=useState(""), [selection, setSelection] = useState<{ operation: Operation; row?: Entity }>();
  const scope = operatorID ? `/api/v1/operators/${encodeURIComponent(operatorID)}` : "/api/v1/me";
  const parameters = new URLSearchParams(); if (cursor) parameters.set("cursor", cursor); if (search) parameters.set("q", search);if(view)parameters.set("view",view);
  const path = `${scope}/${resource}${parameters.size ? `?${parameters}` : ""}`;
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const canRead = customer ? session?.kind === "customer" : !!workspace && (workspace.permissions.includes("*") || workspace.permissions.includes(`${resource}.read`));
  const data = useQuery({ queryKey: ["resource", scope, resource, cursor, search,view], queryFn: () => request(path, ResourcePageSchema), enabled: canRead });
  const canOperate = (operation: Operation) => !operation.permission || !!workspace?.permissions.includes("*") || !!workspace?.permissions.includes(operation.permission);
  if (!canRead) return <Empty title="没有查看权限">此页面需要当前经营方明确授予相应权限。</Empty>;
  if (data.isPending) return <Loading />;
  if (data.isError) return <><Heading title={resourceLabel(resource)} /><ErrorPanel error={data.error} retry={() => void data.refetch()} /></>;
  const page = data.data;
 const resourceName=resource.split("/")[0];
  return <><Heading title={page.title===resourceName?resourceLabel(resourceName):page.title} description={page.description} actions={<div className="flex flex-wrap gap-2">{page.operations.filter(canOperate).map(operation => <Button key={operation.name} type="button" onClick={() => setSelection({ operation })}>{operation.label}</Button>)}</div>} />
    {page.views&&<div className="mb-5 flex gap-3">{page.views.map(item=><Button key={item.value} variant={(view||page.views?.[0]?.value)===item.value?"default":"outline"} onClick={()=>{setView(item.value);setCursor("");setCursorHistory([])}}>{item.label}</Button>)}</div>}
    {page.summary && <div className="mb-7 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{page.summary.map((item, i) => <Stat key={i} {...item} />)}</div>}
    {page.availability?.filter(item => !item.enabled).map(item => <div key={item.capability} className="mb-4"><Notice>{item.reason || "相应业务暂未开放"}{item.next_action && <p>{item.next_action}</p>}</Notice></div>)}
    {selection && <OperationForm key={`${selection.operation.name}:${selection.row?.id || "new"}`} operation={selection.operation} row={selection.row} operatorID={operatorID} onClose={() => setSelection(undefined)} />}
    {invoiceID&&<InvoiceFiles scope={`${scope}/${resourceName}`} invoiceID={invoiceID} upload={!customer&&resourceName==="invoices"} close={()=>setInvoiceID("")}/>}
    <form className="mb-5 flex gap-3" onSubmit={event => { event.preventDefault(); setCursor(""); setCursorHistory([]); setSearch(query); }}><Input aria-label="搜索记录" placeholder="搜索业务编号、名称或关联记录" value={query} onChange={event => setQuery(event.target.value)} className="max-w-lg" /><Button variant="outline" type="submit">查询</Button></form>
    <Table rows={page.items} getID={row => row.id} empty="当前筛选下没有记录。实际业务产生后在此追踪。" columns={[...page.columns.map(column => ({ label: column.label, cell: (row: Entity) => <Value value={column.key === "id" ? row.id : column.key === "status" ? row.status || null : row.values[column.key]} kind={column.kind} /> })), ...(["invoices","procurement-invoices"].includes(resourceName)?[{label:"私有票据",cell:(row:Entity)=><Button variant="outline" size="sm" onClick={()=>setInvoiceID(row.id)}>文件与下载</Button>}]:[]), ...(page.row_operations.length ? [{ label: "操作", cell: (row: Entity) => <div className="flex flex-wrap gap-2">{page.row_operations.filter(operation => canOperate(operation) && (!row.allowed_actions || row.allowed_actions.includes(operation.name))).map(operation => <Button key={operation.name} variant="outline" size="sm" onClick={() => setSelection({ operation, row })}>{operation.label}</Button>)}</div> }] : [])]} />
    <div className="mt-5 flex justify-end gap-3">{cursorHistory.length > 0 && <Button variant="outline" onClick={() => { const previous = [...cursorHistory]; setCursor(previous.pop() || ""); setCursorHistory(previous); }}>上一页</Button>}{page.next_cursor && <Button variant="outline" onClick={() => { setCursorHistory(previous => [...previous, cursor]); setCursor(page.next_cursor || ""); }}>下一页</Button>}</div>
  </>;
}
function Value({ value, kind }: { value?: JSONValue; kind: string }) { if (value === null || value === undefined) return <span className="text-ink-mute">—</span>; if(kind==="link"&&typeof value==="string"&&(value.startsWith("/manage/")||value.startsWith("/api/v1/operators/")))return <a className="text-brand-emphasis underline" href={value}>进入业务办理</a>;if(kind==="details")return <details><summary className="cursor-pointer text-brand-emphasis">查看已保存事实</summary><div className="mt-3"><ReadFacts value={value}/></div></details>;if (kind === "usd" || kind === "atg") return <Money value={String(value)} unit={kind.toUpperCase()} />; if (kind === "datetime") return <When value={String(value)} />; if (kind === "status") return <Status value={String(value)} />; if (kind === "boolean") return <span>{value ? "是" : "否"}</span>; return <span className="whitespace-pre-wrap break-words">{typeof value === "object" ? JSON.stringify(value) : String(value)}</span>; }

function ReadFacts({value}:{value:JSONValue}){if(value!==null&&typeof value==="object")return <dl className="space-y-2">{Object.entries(value).map(([key,item])=><div key={key}><dt className="text-xs text-ink-mute">{key}</dt><dd className="ml-2 break-all text-xs"><ReadFacts value={item}/></dd></div>)}</dl>;return <span>{String(value??"未记录")}</span>}
