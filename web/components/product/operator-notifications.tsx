"use client";
import { useI18n } from "@/components/i18n";
import { useState } from "react";
import { X } from "lucide-react";
import * as Dialog from "@radix-ui/react-dialog";
import type { Entity, JSONValue, Operation } from "@/lib/contracts";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Empty, Status, When } from "./shared";

type Notice = { id: string; title: string; body: string; recipient_name: string; recipient_email: string; recipient_id: string; read_status: string; state: string; created_at: string; system_generated: boolean };

function text(value: JSONValue | undefined) { return typeof value === "string" ? value : ""; }
function count(value: JSONValue | undefined) { return typeof value === "number" ? value : 0; }
function noticesOf(row: Entity): Notice[] {
  const raw = row.values.notices;
  if (!Array.isArray(raw)) return [];
  return raw.flatMap(item => {
    if (!item || typeof item !== "object" || Array.isArray(item)) return [];
    const notice = item as Record<string, JSONValue>;
    return [{ id: text(notice.id), title: text(notice.title), body: text(notice.body), recipient_name: text(notice.recipient_name), recipient_email: text(notice.recipient_email), recipient_id: text(notice.recipient_id), read_status: text(notice.read_status), state: text(notice.state), created_at: text(notice.created_at), system_generated: notice.system_generated === true }];
  });
}
function copy(system: boolean, value: string, t: (source: string) => string) { return system ? t(value) : value; }

export function OperatorNotificationGroups({ page, onRetry }: { page: { items: Entity[]; row_operations: Operation[] }; onRetry: (noticeID: string) => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState<Entity>();
  const personal = page.items.find(item => item.values.record_kind === "personal");
  const events = page.items.filter(item => item.values.record_kind === "event");
  const retry = page.row_operations.some(operation => operation.name === "retry");
  const selected = open ? noticesOf(open) : [];
  const event = open?.values.record_kind === "event";
  return <>
    <div className="space-y-3">
      {personal && <Card><div className="flex flex-wrap items-center justify-between gap-4"><div><h2 className="font-semibold">{t("发给我的通知")}</h2><p className="mt-2 text-sm text-ink-secondary">{t("未读 {count} 条", { count: count(personal.values.unread_count) })}</p></div><Button variant="outline" onClick={() => setOpen(personal)}>{t("查看详情")}</Button></div></Card>}
      {events.map(item => <Card key={item.id}><div className="flex flex-wrap items-center justify-between gap-4"><div className="min-w-0"><h2 className="font-semibold">{copy(item.values.system_generated === true, text(item.values.title), t)}</h2><p className="mt-2 text-xs text-ink-mute"><When value={text(item.values.created_at)} /></p><p className="mt-2 text-sm text-ink-secondary">{t("未读 {unread} 人 / 共 {total} 人", { unread: count(item.values.unread_count), total: count(item.values.total_count) })}</p></div><Button variant="outline" onClick={() => setOpen(item)}>{t("查看详情")}</Button></div></Card>)}
      {!events.length && <Empty title={t("还没有发给其他人的通知事件")}>{t("同一次发送会合成一组。同一事项的两次发送分别成组。")}</Empty>}
    </div>
    <Dialog.Root open={!!open} onOpenChange={next => { if (!next) setOpen(undefined); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content className="th-operation-panel fixed inset-y-0 right-0 z-50 w-full max-w-3xl overflow-y-auto border-l border-hairline bg-canvas-raised p-5 focus:outline-none sm:p-8">
          <div className="mb-6 flex items-start justify-between gap-4 border-b border-hairline pb-5"><div className="min-w-0 pr-8"><Dialog.Title className="text-xl font-semibold">{event ? copy(open?.values.system_generated === true, text(open?.values.title), t) : t("发给我的通知")}</Dialog.Title><Dialog.Description className="mt-2 text-sm leading-6 text-ink-mute">{event ? t("未读的人排在前面，已读的人排在后面。") : t("未读通知排在前面，已读通知排在后面。")}</Dialog.Description></div><Dialog.Close asChild><Button variant="ghost" size="icon" aria-label={t("收起详情")}><X /></Button></Dialog.Close></div>
          {event && <p className="mb-6 whitespace-pre-wrap text-sm leading-7 text-ink-secondary">{copy(open?.values.system_generated === true, text(open?.values.body), t)}</p>}
          <div className="space-y-3">{selected.map(notice => <div key={notice.id} className="rounded-control border border-hairline p-4"><div className="flex flex-wrap items-start justify-between gap-3"><div className="min-w-0"><h3 className="font-medium">{event ? (notice.recipient_name || notice.recipient_email || notice.recipient_id) : copy(notice.system_generated, notice.title, t)}</h3>{event && notice.recipient_email && <p className="mt-1 text-xs text-ink-mute">{notice.recipient_email}</p>}{!event && <p className="mt-2 whitespace-pre-wrap text-sm leading-7 text-ink-secondary">{copy(notice.system_generated, notice.body, t)}</p>}</div><div className="flex flex-wrap items-center gap-2"><Status value={notice.read_status} />{notice.state && <Status value={notice.state} />}</div></div><div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-ink-mute"><When value={notice.created_at} />{retry && notice.state === "needs_attention" && <Button size="sm" variant="outline" onClick={() => onRetry(notice.id)}>{t("重试原通知")}</Button>}</div></div>)}{!selected.length && <Empty title={event ? t("这次通知没有收件人") : t("还没有发给你的通知")} />}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  </>;
}
