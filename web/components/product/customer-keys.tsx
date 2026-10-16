"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { request } from "@/lib/api-client";
import { collection, KeySchema, KeySecretSchema, ModelSchema, CommandResultSchema, Decimal, type APIKey, type KeyInput } from "@/lib/contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { Empty, ErrorPanel, Heading, Loading, Money, Notice, Status, Table, When, useCommand } from "./shared";

export function CustomerKeys() {
  const [editing, setEditing] = useState<APIKey | null | undefined>();
  const data = useQuery({ queryKey: ["keys"], queryFn: () => request("/api/v1/me/keys", collection(KeySchema)) });
  return <><Heading title="API Key" description="多个 Key 共享本账号额度。预算只限制访问和使用，不提前划拨资产；重置密钥不会清零当前窗口的已用和在途预占。" actions={<Button onClick={() => setEditing(null)}>创建 Key</Button>} />
    {editing !== undefined && <KeyEditor key={editing?.id || "new"} existing={editing || undefined} close={() => setEditing(undefined)} />}
    {data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <Table rows={data.data.items} getID={key => key.id} empty="为不同应用创建 Key，分别管理模型范围、预算和并发。完整密钥只展示一次。" columns={[
      { label: "名称与标识", cell: key => <div><p className="font-medium">{key.name}</p><p className="mt-2 font-mono text-xs text-ink-mute">{key.prefix}…</p></div> }, { label: "状态", cell: key => <Status value={key.status} /> },
      { label: "预算", cell: key => <div><p>{key.budget.limit_atg ? <Money value={key.budget.limit_atg} /> : "不额外限额"}</p><p className="mt-2 text-xs text-ink-mute">{{ lifetime: "不限时累计", month: "自然月", quarter: "自然季", year: "自然年" }[key.budget.period]} · {key.budget.timezone}</p><p className="mt-2 text-xs">已用 <Money value={key.budget.spent_atg} /> · 占用 <Money value={key.budget.held_atg} /></p>{key.budget.overage_atg !== "0" && <p className="mt-2 text-xs text-hold">超额 <Money value={key.budget.overage_atg} /></p>}</div> },
      { label: "范围与限制", cell: key => <div className="space-y-2 text-xs"><p>{key.model_ids === null ? "所有当前已授权并上架模型" : key.model_ids.join("、") || "无模型访问"}</p><p>{key.rate_limit_rpm === null ? "不额外限频" : `${key.rate_limit_rpm} 次/分钟`} · {key.concurrency_limit === null ? "不额外限并发" : `${key.concurrency_limit} 并发`}</p></div> },
      { label: "创建时间", cell: key => <When value={key.created_at} /> },
      { label: "操作", cell: key => key.status === "deleted" ? "历史保留" : <div className="space-y-3"><Button size="sm" variant="outline" onClick={() => setEditing(key)}>编辑配置</Button><KeyActions apiKey={key} /></div> },
    ]} />}
  </>;
}
function KeyEditor({ existing, close }: { existing?: APIKey; close: () => void }) {
  const models = useQuery({ queryKey: ["my-models"], queryFn: () => request("/api/v1/me/models", collection(ModelSchema)) });
  const [name, setName] = useState(existing?.name || ""), [allModels, setAllModels] = useState(existing?.model_ids === null || !existing), [selected, setSelected] = useState<string[]>(existing?.model_ids || []), [limit, setLimit] = useState(existing?.budget.limit_atg || ""), [period, setPeriod] = useState<KeyInput["budget"]["period"]>(existing?.budget.period || "month"), [rpm, setRPM] = useState(existing?.rate_limit_rpm?.toString() || ""), [concurrency, setConcurrency] = useState(existing?.concurrency_limit?.toString() || ""), [validation, setValidation] = useState<string>();
  const create = useCommand("/api/v1/me/keys", KeySecretSchema);
  const update = useCommand(`/api/v1/me/keys/${existing?.id || "new"}`, KeySchema, "PATCH");
  const command = existing ? update : create;
  function counter(value: string) { if (!value) return null; const n = z.coerce.number().int().positive().safe().parse(value); return n; }
  return <Card className="mb-7"><div className="mb-5 flex justify-between"><h2 className="text-lg font-semibold">{existing ? "编辑 Key 配置" : "创建新的 API Key"}</h2><Button variant="ghost" size="sm" onClick={close}>关闭</Button></div><form className="space-y-5" onSubmit={async event => { event.preventDefault(); setValidation(undefined); try { const body = { name, model_ids: allModels ? null : selected, budget: { limit_atg: limit ? Decimal.parse(limit) : null, period }, rate_limit_rpm: counter(rpm), concurrency_limit: counter(concurrency), ...(existing ? { version: existing.version } : {}) }; if (existing) await update.run(body); else await create.run(body); } catch (e) { setValidation(e instanceof Error ? e.message : "请检查配置"); } }}>
    <fieldset disabled={command.busy || command.unresolved} className="space-y-5"><label className="block text-sm">名称<Input value={name} onChange={e => setName(e.target.value)} required maxLength={80} className="mt-2" /></label><div className="grid gap-5 sm:grid-cols-2"><label className="block text-sm">ATG 预算上限<Input value={limit} onChange={e => setLimit(e.target.value)} inputMode="decimal" placeholder="留空表示不额外限额" className="mt-2" /></label><label className="block text-sm">预算窗口<select className="mt-2 h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3" value={period} onChange={e => setPeriod(e.target.value as KeyInput["budget"]["period"])}><option value="lifetime">不限时累计</option><option value="month">自然月</option><option value="quarter">自然季</option><option value="year">自然年</option></select></label><label className="block text-sm">每分钟请求上限<Input inputMode="numeric" value={rpm} onChange={e => setRPM(e.target.value)} className="mt-2" placeholder="留空表示不额外限制" /></label><label className="block text-sm">并发上限<Input inputMode="numeric" value={concurrency} onChange={e => setConcurrency(e.target.value)} className="mt-2" placeholder="留空表示不额外限制" /></label></div><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={allModels} onChange={e => setAllModels(e.target.checked)} />不额外限定模型</label>{!allModels && <div>{models.isPending ? <Loading /> : models.isError ? <ErrorPanel error={models.error} retry={() => void models.refetch()} /> : !models.data.items.length ? <Empty title="当前没有可授权模型" /> : <fieldset className="grid gap-3 sm:grid-cols-2"><legend className="mb-3 text-sm">允许访问的模型</legend>{models.data.items.map(model => <label key={model.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={selected.includes(model.id)} onChange={e => setSelected(previous => e.target.checked ? [...previous, model.id] : previous.filter(id => id !== model.id))} />{model.name}</label>)}</fieldset>}</div>}</fieldset>
    <Notice>预算按经营时区的自然周期计算。修改限额、停用或重置都不清零当前已用金额；已受理请求仍按原规则结算。</Notice>{validation && <p role="alert" className="text-sm text-danger">{validation}</p>}{!!command.error && <ErrorPanel error={command.error} />}{command.unresolved && <Notice>操作结果待核实，再次提交会重试原请求。</Notice>}<Button type="submit" disabled={command.busy}>{command.busy ? "提交中…" : command.unresolved ? "重试本次操作" : existing ? "保存配置" : "创建 Key"}</Button>{create.result && <Secret secret={create.result.secret} />}{update.result && <Notice>配置已保存，Key 历史和预算记录继续保留。</Notice>}
  </form></Card>;
}
function KeyActions({ apiKey }: { apiKey: APIKey }) {
  const [action, setAction] = useState<"reset" | "disable" | "enable" | "delete">();
  return <div><div className="flex flex-wrap gap-2"><button className="text-xs text-brand-emphasis underline" onClick={() => setAction("reset")}>重置密钥</button><button className="text-xs text-brand-emphasis underline" onClick={() => setAction(apiKey.status === "disabled" ? "enable" : "disable")}>{apiKey.status === "disabled" ? "恢复" : "停用"}</button><button className="text-xs text-danger underline" onClick={() => setAction("delete")}>删除</button></div>{action && <KeyAction key={action} apiKey={apiKey} action={action} close={() => setAction(undefined)} />}</div>;
}
function KeyAction({ apiKey, action, close }: { apiKey: APIKey; action: "reset" | "disable" | "enable" | "delete"; close: () => void }) {
  const reset = useCommand(`/api/v1/me/keys/${apiKey.id}/reset`, KeySecretSchema);
  const change = useCommand(`/api/v1/me/keys/${apiKey.id}${action === "delete" ? "" : `/${action}`}`, CommandResultSchema, action === "delete" ? "DELETE" : "POST");
  const command = action === "reset" ? reset : change;
  const text = { reset: "重置后旧密钥立即停止受理新请求，配置与当前预算保留。", disable: "停用后停止受理新请求，已受理请求继续结算。", enable: "恢复后继续使用当前预算记录。", delete: "删除后永久失效，必要账务历史继续保留。" }[action];
  return <div className="mt-4 space-y-3 rounded-control border border-hairline p-3"><p className="text-xs leading-6">{text}</p>{!!command.error && <ErrorPanel error={command.error} />}<div className="flex gap-2"><Button size="sm" variant={action === "delete" ? "destructive" : "outline"} disabled={command.busy} onClick={() => void (action === "reset" ? reset.run({ version: apiKey.version }) : change.run({ version: apiKey.version }))}>{command.busy ? "处理中…" : command.unresolved ? "重试原操作" : "确认"}</Button><Button size="sm" variant="ghost" onClick={close}>关闭</Button></div>{reset.result && <Secret secret={reset.result.secret} />}{change.result && <p className="text-xs text-success">操作已完成</p>}</div>;
}
function Secret({ secret }: { secret: string }) {
  const [copied, setCopied] = useState(false), [error, setError] = useState(false);
  return <div className="rounded-card border border-hold/40 bg-canvas p-4"><p className="mb-3 text-sm font-medium text-hold">完整密钥仅本次显示，请立即保存。</p><code className="block break-all text-xs">{secret}</code><Button type="button" variant="outline" size="sm" className="mt-3" onClick={async () => { try { await navigator.clipboard.writeText(secret); setCopied(true); } catch { setError(true); } }}>{copied ? "已复制" : "复制密钥"}</Button>{error && <p role="alert" className="mt-2 text-xs text-danger">复制失败，请手动选中保存。</p>}</div>;
}
