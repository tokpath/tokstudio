"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
import { ConsolePageHeader } from "@/components/console/page-header";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { notifyInboxChanged } from "./bell";
import { notificationCopy } from "@/lib/system-copy";

type Item = { id: string; kind: string; title: string; body: string; href: string; created_at: string; read_at: string | null; email_status: string };
type Inbox = { items: Item[]; unread: number; next_cursor: string };

export function NotificationInbox() {
  const tUI = useTranslations();
  const t = useTranslations("notifications"); const locale = useLocale();
  const viewer = useViewer(); const brand = useBrand();
  const scope = `${viewer.userId || ""}:${brand?.id || ""}`;
  const [unreadOnly, setUnreadOnly] = useState(false); const [cursor, setCursor] = useState("");
  const [busy, setBusy] = useState(false); const [error, setError] = useState("");
  const [emailOverride, setEmailOverride] = useState<boolean | null>(null);
  useEffect(() => { setCursor(""); setError(""); setEmailOverride(null); }, [scope]);
  const query = useListResource<Inbox>({ enabled: viewer.signedIn && !viewer.loading, queryKey: `${scope}:${unreadOnly}:${cursor}`, load: async () => {
    const params = new URLSearchParams(); if (unreadOnly) params.set("unread", "1"); if (cursor) params.set("before", cursor);
    const response = await fetch(`${apiBase}/v1/me/notifications?${params}`, { credentials: "include" }); const body = await response.json();
    const valid = response.ok && Array.isArray(body.items) && typeof body.unread === "number";
    return { ok: valid, status: response.status, items: valid ? [body] : [] };
  } });
  const prefs = useListResource<{ email_enabled: boolean; smtp_available: boolean }>({ enabled: viewer.signedIn && !viewer.loading, queryKey: scope, load: async () => {
    const response = await fetch(`${apiBase}/v1/me/notification-preferences`, { credentials: "include" }); const body = await response.json();
    return { ok: response.ok, status: response.status, items: response.ok ? [body] : [] };
  } });
  const data = query.snapshot.items[0]; const preference = prefs.snapshot.items[0];
  const items = data?.items.map(item => notificationCopy(item, locale, tUI)) ?? [];
  async function action(path: string, method = "POST", body: object = {}) {
    if (busy) return false; setBusy(true); setError("");
    try { const response = await fetch(`${apiBase}${path}`, { method, credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }); if (!response.ok) throw new Error();
      await Promise.all([query.reload(), prefs.reload()]); notifyInboxChanged(); return true;
    } catch { setError(t("actionFailed")); return false; } finally { setBusy(false); }
  }
  async function changeEmail(enabled: boolean) {
    const previous = emailOverride ?? preference?.email_enabled ?? true;
    setEmailOverride(enabled);
    if (!await action("/v1/me/notification-preferences", "PUT", { email_enabled: enabled })) setEmailOverride(previous);
  }
  return <div className="space-y-6">
    <ConsolePageHeader title={t("title")} description={t("lead")} actions={<Button variant="outline" disabled={busy || query.refreshing} onClick={() => void query.reload()}>{t("refresh")}</Button>} />
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex gap-2" role="group" aria-label={t("filter")}>
          <Button variant={unreadOnly ? "outline" : "default"} size="sm" aria-pressed={!unreadOnly} onClick={() => { setUnreadOnly(false); setCursor(""); }}>{t("all")}</Button>
          <Button variant={unreadOnly ? "default" : "outline"} size="sm" aria-pressed={unreadOnly} onClick={() => { setUnreadOnly(true); setCursor(""); }}>{t("unread", { count: data?.unread ?? 0 })}</Button>
        </div>
        <Button variant="ghost" size="sm" disabled={busy || !data?.unread} onClick={() => void action("/v1/me/notifications/read-all")}>{t("readAll")}</Button>
      </div>
      {error && <p role="alert" className="mt-4 text-sm text-danger">{error}</p>}
      {query.snapshot.phase === "stale" && <p role="alert" className="mt-4 text-sm text-danger">{t("loadFailed")}</p>}
      {!data ? <p role={query.snapshot.phase === "loading" ? "status" : "alert"} className="py-8 text-sm">{t(query.snapshot.phase === "loading" ? "loading" : "loadFailed")}</p> : !data.items.length ? <p className="py-8 text-sm text-ink-secondary">{t(unreadOnly ? "emptyUnread" : "empty")}</p> : <ul className="mt-4 divide-y divide-hairline">{items.map(item => <li key={item.id} className="py-5">
        <div className="flex flex-wrap items-center justify-between gap-2"><h2 className="text-base font-semibold">{!item.read_at && <span className="mr-2 inline-block size-2 rounded-full bg-brand" aria-label={t("new")} />}{item.title}</h2><time dateTime={item.created_at} className="text-xs text-ink-mute">{new Date(item.created_at).toLocaleString(locale, { timeZoneName: "short" })}</time></div>
        <p className="mt-3 whitespace-pre-wrap text-sm leading-relaxed text-ink-secondary">{item.body}</p>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          {item.href.startsWith("/") && !item.href.startsWith("//") && <Button asChild variant="outline" size="sm"><Link href={item.href}>{t("view")}</Link></Button>}
          {!item.read_at && <Button variant="ghost" size="sm" disabled={busy} onClick={() => void action(`/v1/me/notifications/${encodeURIComponent(item.id)}/read`)}>{t("markRead")}</Button>}
          <span className="text-xs text-ink-mute">{t("emailState", { state: t.has(`status_${item.email_status}`) ? t(`status_${item.email_status}`) : item.email_status })}</span>
        </div>
      </li>)}</ul>}
      {(cursor || data?.next_cursor) && <div className="mt-4 flex gap-3"><Button variant="outline" size="sm" disabled={!cursor || busy} onClick={() => setCursor("")}>{t("latest")}</Button><Button variant="outline" size="sm" disabled={!data?.next_cursor || busy} onClick={() => setCursor(data?.next_cursor || "")}>{t("older")}</Button></div>}
    </Card>
    <Card><h2 className="font-semibold">{t("preferences")}</h2>
      {preference ? <><label className="mt-4 flex items-center gap-3 text-sm"><input type="checkbox" checked={emailOverride ?? preference.email_enabled} disabled={busy} onChange={event => void changeEmail(event.target.checked)} />{t("emailEnabled")}</label><p className="mt-3 text-sm text-ink-secondary">{t("preferenceHint")}</p>{!preference.smtp_available && <p className="mt-3 text-sm text-ink-secondary">{t("smtpUnavailable")}</p>}<Button asChild variant="link" size="sm" className="mt-2 px-0"><Link href="/app/settings">{t("verifyEmail")}</Link></Button></> : <p className="mt-3 text-sm">{t(prefs.snapshot.phase === "loading" ? "loading" : "loadFailed")}</p>}
    </Card>
  </div>;
}
