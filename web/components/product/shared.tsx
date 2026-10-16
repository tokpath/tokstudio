"use client";
import { type ReactNode, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { APIError, request, newActionKey } from "@/lib/api-client";
import { previewDecimal, formatDecimal } from "@/lib/decimal";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

export function Heading({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description?: string; actions?: ReactNode }) { return <div className="mb-8 flex flex-wrap items-start justify-between gap-4"><div>{eyebrow && <p className="th-eyebrow mb-3 text-brand-emphasis">{eyebrow}</p>}<h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">{title}</h1>{description && <p className="mt-3 max-w-3xl text-sm leading-7 text-ink-secondary">{description}</p>}</div>{actions}</div>; }
export function Money({ value, unit = "ATG" }: { value: string; unit?: string }) { return <span title={`${value} ${unit} · 双击复制精确值`} aria-label={`${value} ${unit}`} onDoubleClick={() => void navigator.clipboard?.writeText(value)} className="font-mono tabular-nums">{previewDecimal(value)} <span className="text-xs text-ink-mute">{unit}</span></span>; }
export function When({ value }: { value?: string | null }) { if (!value) return <span>—</span>; const date = new Date(value); return <time dateTime={value} title={value}>{Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date)}</time>; }
const statusLabels: Record<string, string> = { awaiting_payment:"待付款",requested:"已申请待办理",review_required:"须重新复核",voided:"已作废",prepared:"已预留待执行",manual_executing:"已登记意图待核实",executing:"执行中",awaiting_usage:"用量待核实",submitted:"已提交原任务",settled:"已结算",exception:"异常待复核",verified_pending_settlement:"事实已核实，责任待清偿",pending_review:"待审批",stopped:"已停止使用",closing:"注销清算中",suspended:"已暂停后续分佣",terminated:"已终止后续分佣",needs_confirmation:"需要本人重确认",notice_late:"通知迟到待重排",application_requires_review:"生效待复核",pending: "待处理", pending_payment: "待付款", partially_paid: "部分付款", paid: "足额待交付", paid_pending_delivery: "足额待交付", fulfilled: "已交付", completed: "已完成", succeeded: "已成功", failed: "失败", dead: "重试耗尽", active: "有效", enabled: "已启用", disabled: "已停用", deleted: "已删除", expired: "已到期", canceled: "已取消", cancelled: "已取消", closed: "已关闭", running: "执行中", processing: "处理中", pending_verification: "待核实", unknown: "结果待核实", frozen: "冻结", available: "可用", held: "已占用", pending_configuration: "待配置", published: "已发布", draft: "草稿", ready: "已就绪", approved: "已批准待执行", paid_out: "已核实付款", refunded: "已核实退款", pending_confirmation: "待确认", restricted: "受限", qualified: "已取得资格", normal: "正常", in_progress: "处理中" };
export function Status({ value }: { value: string }) { const bad = ["failed", "dead", "restricted"].includes(value); const good = ["succeeded", "fulfilled", "completed", "active", "published", "available", "ready"].includes(value); return <span className={`inline-flex rounded-full border px-2.5 py-1 text-xs ${bad ? "border-danger/30 text-danger" : good ? "border-success/30 text-success" : "border-hairline text-ink-secondary"}`}>{statusLabels[value] || value}</span>; }
export function Loading() { return <div role="status" className="animate-pulse rounded-card border border-hairline bg-canvas-raised p-8 text-sm text-ink-mute">正在加载当前记录…</div>; }
export function Empty({ title = "暂无记录", children }: { title?: string; children?: ReactNode }) { return <div className="rounded-card border border-dashed border-hairline px-6 py-12 text-center"><h3 className="font-medium">{title}</h3>{children && <div className="mx-auto mt-2 max-w-xl text-sm leading-7 text-ink-mute">{children}</div>}</div>; }
export function ErrorPanel({ error, retry, compact = false }: { error: unknown; retry?: () => void; compact?: boolean }) {
  const e = error instanceof Error ? error : new Error("暂时无法完成请求。");
  const api = error instanceof APIError ? error : undefined;
  return <div role="alert" className={`rounded-card border border-danger/30 bg-canvas-raised ${compact ? "p-3" : "p-5"}`}><p className="text-sm font-medium text-danger">{e.message}</p>{api?.nextAction && <p className="mt-2 text-sm text-ink-secondary">{api.nextAction}</p>}{api?.requestId && <p className="mt-2 font-mono text-xs text-ink-mute">请求编号 {api.requestId} · {api.code}</p>}{retry && <Button type="button" variant="outline" size="sm" className="mt-3" onClick={retry}>重试读取</Button>}</div>;
}
export function Notice({ children }: { children: ReactNode }) { return <div className="rounded-card border border-hairline bg-brand-soft/30 px-5 py-4 text-sm leading-7 text-ink-secondary">{children}</div>; }
export function Stat({ label, value, unit, detail }: { label: string; value: string; unit?: string; detail?: string }) { return <Card><p className="text-sm text-ink-secondary">{label}</p><p className="mt-3 break-words text-2xl font-semibold">{unit ? <Money value={value} unit={unit} /> : value}</p>{detail && <p className="mt-3 text-xs leading-6 text-ink-mute">{detail}</p>}</Card>; }
export function Table<T>({ rows, columns, getID, empty }: { rows: T[]; columns: { label: string; cell: (row: T) => ReactNode }[]; getID: (row: T) => string; empty?: string }) { return rows.length ? <div className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised"><table className="w-full min-w-[640px] text-left text-sm"><thead><tr className="border-b border-hairline">{columns.map((column, i) => <th className="px-5 py-4 font-medium text-ink-mute" key={i}>{column.label}</th>)}</tr></thead><tbody>{rows.map(row => <tr key={getID(row)} className="border-b border-hairline last:border-0">{columns.map((column, i) => <td className="max-w-sm px-5 py-4 align-top" key={i}>{column.cell(row)}</td>)}</tr>)}</tbody></table></div> : <Empty>{empty}</Empty>; }

// A network-ambiguous attempt keeps its exact body and UUID until queried or
// retried. Successful actions clear the attempt so the next action gets a new ID.
export function useCommand<T extends z.ZodTypeAny>(path: string, schema: T, method: "POST" | "PATCH" | "DELETE" = "POST") {
  const cache = useQueryClient();
  const attempt = useRef<{ key: string; fingerprint: string; body: unknown } | undefined>(undefined);
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [result, setResult] = useState<z.infer<T>>();
  const unresolved = error instanceof APIError && (error.retryable || error.code === "contract_mismatch");
  async function run(body: unknown) {
    if (busy) return;
    const fingerprint = body instanceof FormData ? Array.from(body.entries()).map(([key, value]) => `${key}:${typeof value === "string" ? value : `${value.name}:${value.size}:${value.lastModified}`}`).join("|") : JSON.stringify(body);
    if (!attempt.current || (!unresolved && attempt.current.fingerprint !== fingerprint)) attempt.current = { key: newActionKey(), fingerprint, body };
    const current = attempt.current;
    setBusy(true); setError(undefined);
    try { const value = await request(path, schema, { method, body: current.body, actionKey: current.key }); setResult(value); attempt.current = undefined; await cache.invalidateQueries(); return value; }
    catch (e) { setError(e); return undefined; }
    finally { setBusy(false); }
  }
  return { run, busy, error, result, unresolved, actionKey: attempt.current?.key };
}
