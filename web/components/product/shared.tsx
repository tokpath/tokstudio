"use client";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { APIError, request, newActionKey } from "@/lib/api-client";
import { previewDecimal, formatDecimal } from "@/lib/decimal";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useI18n } from "@/components/i18n";
import { localeTags, localizeNumberText } from "@/lib/i18n";
import { FieldHelp } from "./field-help";
import { Inbox } from "lucide-react";
import { useMonetary } from "./monetary";

export function Heading({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description?: string; actions?: ReactNode }) { const { t } = useI18n(); return <div className="th-page-heading"><div>{eyebrow && eyebrow !== title && <p className="th-eyebrow text-brand-emphasis">{eyebrow}</p>}<h1>{t(title)}</h1>{description && <p className="mt-2 max-w-3xl text-sm leading-6 text-ink-secondary">{t(description)}</p>}</div>{actions && <div className="flex shrink-0 flex-wrap items-center gap-2 sm:pt-1">{actions}</div>}</div>; }
export function Money({ value, unit = "ATG" }: { value: string; unit?: string }) { const { t, locale } = useI18n(); const monetary = useMonetary(); const currency = unit === "MONEY" ? monetary.currency : unit; return <span title={t("{value} {unit} · 双击复制精确值", {value, unit: currency})} aria-label={`${value} ${currency}`} onDoubleClick={() => void navigator.clipboard?.writeText(value)} className="th-money tabular-nums [overflow-wrap:anywhere]">{localizeNumberText(previewDecimal(value), locale)} <span className="text-xs font-normal tracking-normal text-ink-mute">{currency}</span></span>; }
export function When({ value }: { value?: string | null }) { const { locale } = useI18n(); if (!value) return <span>—</span>; const date = new Date(value); return <time dateTime={value} title={value}>{Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(localeTags[locale], { dateStyle: "medium", timeStyle: "short" }).format(date)}</time>; }
export const statusLabels: Record<string, string> = {not_synced:"尚未同步",stale:"数据已过期",awaiting_settlement:"待核实结算金额",scheduled:"未开始",ended:"已结束",archived:"已归档",awaiting_manual_payment:"等待线下付款",outcome_unknown:"付款结果待核实",verified_payment_delivery_pending:"已收款待交付",cancelled_unpaid:"已取消未付款",failed_unpaid:"付款失败未扣款",needs_attention:"需要处理",recorded:"已登记", saved: "已保存", preparing: "准备中", awaiting_approval: "待独立复核", pending_costs: "成本待核实", confirmed_facts: "事实已确认", sent: "已发送", delivered: "已送达", queued: "等待发送", deferred: "延后发送", suppressed: "不再发送", bounced: "投递失败", warning: "预警", critical: "严重风险", resolved: "已解除", rejected: "已拒绝", authorized: "已授权", procurement_empty: "采购余额不足", uncovered: "履约覆盖不足", sold_out: "暂停售卖", overselling: "已使用超卖额度", replenish_early: "请提前补货", accepted:"已受理", cancellation_requested:"取消待确认", reconciliation_queued:"已提交核实", confirmed:"已核实", allocated:"已分配", quote:"有效报价", authorization:"已授权", true:"已启用", false:"已停用", lifetime:"不限时", month:"每月", quarter:"每季", year:"每年", quoted:"费用待确认", paying:"付款核实中", issued:"已发放", read:"已读", unread:"未读", configured:"已配置", effective:"已生效", calculated:"已核算", updated:"已更新", free:"未收费", published:"已发布", awaiting_payment:"待付款",requested:"已申请待办理",review_required:"须重新复核",voided:"已作废",prepared:"已预留待执行",manual_executing:"已登记意图待核实",executing:"执行中",awaiting_usage:"用量待核实",submitted:"已提交原任务",settled:"已结算",exception:"异常待复核",verified_pending_settlement:"事实已核实，责任待清偿",pending_review:"待审批",stopped:"已停止使用",closing:"注销清算中",suspended:"已暂停后续分佣",terminated:"已终止后续分佣",needs_confirmation:"需要本人重确认",notice_late:"通知迟到待重排",application_requires_review:"生效待复核",pending: "待处理", pending_payment: "待付款", partially_paid: "部分付款", paid: "足额待交付", paid_pending_delivery: "足额待交付", fulfilled: "已交付", completed: "已完成", succeeded: "已成功", failed: "失败", dead: "重试耗尽", active: "有效", enabled: "已启用", disabled: "已停用", deleted: "已删除", expired: "已到期", canceled: "已取消", cancelled: "已取消", closed: "已关闭", running: "执行中", processing: "处理中", pending_verification: "待核实", unknown: "结果待核实", frozen: "冻结", available: "可用", held: "已占用", pending_configuration: "待配置", draft: "草稿", ready: "已就绪", approved: "已批准待执行", paid_out: "已核实付款", refunded: "已核实退款", pending_confirmation: "待确认", restricted: "受限", qualified: "已取得资格", normal: "正常", in_progress: "处理中" };
export function Status({ value }: { value: string }) { const { t } = useI18n(); const bad = ["failed", "dead", "restricted", "critical", "exception"].includes(value); const good = ["succeeded", "fulfilled", "completed", "active", "published", "available", "ready"].includes(value); const pending = ["pending", "pending_payment", "awaiting_payment", "pending_verification", "unknown", "frozen", "held", "awaiting_approval", "pending_review", "overselling", "warning"].includes(value); return <span className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-1 text-xs font-medium ${bad ? "bg-danger/10 text-danger" : good ? "bg-success/10 text-success" : pending ? "bg-hold/10 text-hold" : "bg-canvas text-ink-secondary"}`}><span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-current opacity-75" />{t(statusLabels[value] || value)}</span>; }
export function Loading() { const { t } = useI18n(); return <div role="status" aria-live="polite" className="min-h-36 rounded-card border border-hairline bg-canvas-raised p-6"><span className="sr-only">{t("正在加载当前记录…")}</span><div aria-hidden="true" className="space-y-4 motion-safe:animate-pulse"><div className="h-3 w-32 rounded bg-hairline" /><div className="h-3 w-2/3 rounded bg-hairline/60" /><div className="h-3 w-1/2 rounded bg-hairline/60" /></div></div>; }
export function Empty({ title = "暂无记录", children }: { title?: string; children?: ReactNode }) { const { t } = useI18n(); return <div className="th-empty rounded-card border border-dashed border-hairline bg-canvas-raised px-6 py-12 text-center"><span aria-hidden="true" className="mx-auto mb-4 grid size-10 place-items-center rounded-xl bg-canvas text-ink-mute"><Inbox className="size-5" /></span><h3 className="text-sm font-medium">{t(title)}</h3>{children && <div className="mx-auto mt-2 max-w-xl text-sm leading-7 text-ink-mute">{children}</div>}</div>; }
export function useScrollIntoView<T extends HTMLElement = HTMLElement>(active: unknown, block: ScrollLogicalPosition = "nearest") {
  const ref = useRef<T>(null);
  useEffect(() => {
    if (active === undefined || active === null || active === false || active === "") return;
    ref.current?.scrollIntoView?.({ behavior: "smooth", block });
  }, [active, block]);
  return ref;
}
export function AlertText({ children, className = "text-sm text-danger" }: { children: ReactNode; className?: string }) {
  const ref = useScrollIntoView<HTMLParagraphElement>(children);
  return <p ref={ref} role="alert" className={className}>{children}</p>;
}
export function ErrorPanel({ error, retry, compact = false }: { error: unknown; retry?: () => void; compact?: boolean }) {
  const { t } = useI18n();
  const e = error instanceof Error ? error : new Error("暂时无法完成请求。");
  const api = error instanceof APIError ? error : undefined;
  const ref = useScrollIntoView<HTMLDivElement>(e.message);
  return <div ref={ref} role="alert" className={`rounded-card border border-danger/30 bg-canvas-raised ${compact ? "p-3" : "p-5"}`}><p className="whitespace-pre-line text-sm font-medium text-danger">{t(e.message)}</p>{api?.nextAction && <p className="mt-2 text-sm text-ink-secondary">{t(api.nextAction)}</p>}{api?.requestId && <p className="mt-2 font-mono text-xs text-ink-mute">{t("请求编号")} {api.requestId} · {api.code}</p>}{retry && <Button type="button" variant="outline" size="sm" className="mt-3" onClick={retry}>{t("重试读取")}</Button>}</div>;
}
export function Notice({ children }: { children: ReactNode }) { return <div className="rounded-control border-l-2 border-brand/40 bg-brand-soft/30 px-4 py-3 text-sm leading-7 text-ink-secondary">{children}</div>; }
export function Stat({ label, value, unit, detail, help }: { label: string; value: string; unit?: string; detail?: string; help?:string }) { const { t } = useI18n(); return <Card className="min-w-0"><p className="text-xs font-medium text-ink-secondary">{t(label)}{help && <FieldHelp title={t(label)}>{t(help)}</FieldHelp>}</p><p className="mt-3 break-words text-[28px] font-semibold tracking-tight">{unit ? <Money value={value} unit={unit} /> : value}</p>{detail && <p className="mt-3 text-xs leading-6 text-ink-mute">{t(detail)}</p>}</Card>; }
export function Table<T>({ rows, columns, getID, empty }: { rows: T[]; columns: { label: string; help?: string; align?: "right"; cell: (row: T) => ReactNode }[]; getID: (row: T) => string; empty?: string }) { const { t } = useI18n(); return rows.length ? <div className="th-table th-scrollbar"><table className="w-full min-w-[640px] text-left text-sm"><thead><tr className="border-b border-hairline">{columns.map((column, i) => <th scope="col" className={column.align === "right" ? "text-right" : undefined} key={i}>{t(column.label)}{column.help && <FieldHelp title={t(column.label)}>{t(column.help)}</FieldHelp>}</th>)}</tr></thead><tbody>{rows.map(row => <tr key={getID(row)}>{columns.map((column, i) => <td className={column.align === "right" ? "text-right tabular-nums" : undefined} key={i}>{column.cell(row)}</td>)}</tr>)}</tbody></table></div> : <Empty>{empty ? t(empty) : undefined}</Empty>; }

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
  function reset() { attempt.current = undefined; setResult(undefined); setError(undefined); }
  return { run, reset, busy, error, result, unresolved, actionKey: attempt.current?.key };
}
