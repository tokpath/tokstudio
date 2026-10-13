"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { ConfirmButton } from "@/components/confirm-button";
import { IfRoles } from "@/components/rbac/if-can";
import { useViewer } from "@/components/rbac/viewer-context";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { deliveryTitle } from "@/lib/system-copy";
import { confirmHeaders } from "@/lib/confirm";

type Settings = { enabled: boolean; available: boolean; source: string; host: string; port: number; username: string; from: string; security: string; password_set: boolean };
type Delivery = { id: string; kind?: string; title: string; brand_id: string; status: string; attempts: number; last_error: string; created_at: string; accepted_at?: string };
type Deliveries = { items?: Delivery[] | null; pending: number; blocked: number; failed: number; accepted: number };

function SMTPSettings() {
  const t = useTranslations("notifications"); const viewer = useViewer();
  const [form, setForm] = useState({ enabled: false, host: "", port: 587, username: "", from: "", security: "starttls" });
  const [password, setPassword] = useState(""); const [busy, setBusy] = useState(false); const [error, setError] = useState(""); const [message, setMessage] = useState("");
  const query = useListResource<Settings>({ queryKey: viewer.userId, load: async () => {
    const response = await fetch(`${apiBase}/admin/notifications/smtp`, { credentials: "include" }); const body = await response.json();
    return { ok: response.ok, status: response.status, items: response.ok ? [body.item] : [] };
  } });
  const settings = query.snapshot.items[0];
  useEffect(() => { if (settings) setForm({ enabled: settings.enabled, host: settings.host, port: settings.port, username: settings.username, from: settings.from, security: settings.security }); }, [settings]);
  async function save() {
    setBusy(true); setError(""); setMessage("");
    try { const response = await fetch(`${apiBase}/admin/notifications/smtp`, { method: "PUT", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ ...form, ...(password ? { password } : {}) }) });
      const body = await response.json(); if (!response.ok) throw new Error(body.error?.message || t("actionFailed"));
      setPassword(""); setMessage(t("smtpSaved")); await query.reload(); return true;
    } catch (error) { setError(error instanceof Error ? error.message : t("actionFailed")); return false; } finally { setBusy(false); }
  }
  async function test() {
    setBusy(true); setError(""); setMessage("");
    try { const response = await fetch(`${apiBase}/admin/notifications/smtp/test`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: "{}" });
      const body = await response.json(); if (!response.ok) throw new Error(body.error?.message || t("actionFailed"));
      setMessage(t("testQueued")); window.dispatchEvent(new Event("tokenhub:notification-deliveries-changed"));
    } catch (error) { setError(error instanceof Error ? error.message : t("actionFailed")); } finally { setBusy(false); }
  }
  return <Card><h2 className="text-lg font-semibold">{t("smtpTitle")}</h2><p className="mt-3 text-sm leading-relaxed text-ink-secondary">{t("smtpLead")}</p>
    {!settings ? <p className="mt-4" role={query.snapshot.phase === "loading" ? "status" : "alert"}>{t(query.snapshot.phase === "loading" ? "loading" : "loadFailed")}</p> : <>
      <p className="mt-3 text-sm">{t(settings.available ? "smtpReady" : "smtpNotReady")}</p>
      <label className="mt-4 flex items-center gap-3 text-sm"><input type="checkbox" checked={form.enabled} onChange={event => setForm({ ...form, enabled: event.target.checked })} />{t("smtpEnabled")}</label>
      <div className="mt-5 grid gap-4 sm:grid-cols-2">
        <label className="space-y-2 text-sm">{t("smtpHost")}<Input autoComplete="off" value={form.host} onChange={event => setForm({ ...form, host: event.target.value })} /></label>
        <label className="space-y-2 text-sm">{t("smtpPort")}<Input type="number" min={1} max={65535} value={form.port} onChange={event => setForm({ ...form, port: Number(event.target.value) })} /></label>
        <label className="space-y-2 text-sm">{t("smtpFrom")}<Input type="email" autoComplete="off" value={form.from} onChange={event => setForm({ ...form, from: event.target.value })} /></label>
        <label className="space-y-2 text-sm">{t("smtpSecurity")}<select className="h-10 w-full rounded-control border border-hairline bg-canvas px-3" value={form.security} onChange={event => setForm({ ...form, security: event.target.value })}><option value="starttls">STARTTLS</option><option value="tls">TLS</option></select></label>
        <label className="space-y-2 text-sm">{t("smtpUsername")}<Input autoComplete="off" value={form.username} onChange={event => setForm({ ...form, username: event.target.value })} /></label>
        <label className="space-y-2 text-sm">{t("smtpPassword")}<Input type="password" autoComplete="new-password" value={password} placeholder={t(settings.password_set ? "passwordKept" : "passwordNew")} onChange={event => setPassword(event.target.value)} /></label>
      </div>
      <div className="mt-5 flex flex-wrap gap-3"><ConfirmButton disabled={busy} title={t("smtpSave")} description={t("smtpConfirm")} onConfirm={save}>{t("smtpSave")}</ConfirmButton><Button variant="outline" disabled={busy || !settings.available} onClick={() => void test()}>{t("smtpTest")}</Button><Button asChild variant="link"><Link href="/app/settings">{t("verifyEmail")}</Link></Button></div>
    </>}
    {message && <p role="status" className="mt-4 text-sm">{message}</p>}{error && <p role="alert" className="mt-4 text-sm text-danger">{error}</p>}
  </Card>;
}

export function NotificationDeliveryLog() {
  const tUI = useTranslations();
  const t = useTranslations("notifications"); const locale = useLocale(); const viewer = useViewer();
  const [status, setStatus] = useState(""); const [busy, setBusy] = useState(false); const [error, setError] = useState("");
  const query = useListResource<Deliveries>({ queryKey: `${viewer.userId}:${status}`, load: async () => {
    const response = await fetch(`${apiBase}/admin/notifications/deliveries?status=${encodeURIComponent(status)}`, { credentials: "include" }); const body = await response.json();
    return { ok: response.ok, status: response.status, items: response.ok ? [body] : [] };
  } });
  useEffect(() => { const changed = () => void query.reload(); window.addEventListener("tokenhub:notification-deliveries-changed", changed); return () => window.removeEventListener("tokenhub:notification-deliveries-changed", changed); }, [query.reload]);
  const data = query.snapshot.items[0]; const deliveries = data?.items ?? []; const canRetry = viewer.roles.some(role => ["platform_admin", "tech_admin", "ops_admin"].includes(role));
  async function retry(id: string) {
    setBusy(true); setError("");
    try { const response = await fetch(`${apiBase}/admin/notifications/deliveries/${encodeURIComponent(id)}/retry`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: "{}" }); const body = await response.json(); if (!response.ok) throw new Error(body.error?.message || t("actionFailed")); await query.reload(); }
    catch (error) { setError(error instanceof Error ? error.message : t("actionFailed")); } finally { setBusy(false); }
  }
  return <Card><div className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-lg font-semibold">{t("deliveryTitle")}</h2><Button variant="outline" size="sm" disabled={busy || query.refreshing} onClick={() => void query.reload()}>{t("refresh")}</Button></div>
    <p className="mt-3 text-sm leading-relaxed text-ink-secondary">{t("deliveryHint")}</p>
    {data && <p className="mt-4 text-sm">{t("deliveryCounts", { pending: data.pending, blocked: data.blocked, failed: data.failed, accepted: data.accepted })}</p>}
    <label className="mt-5 flex flex-wrap items-center gap-3 text-sm">{t("deliveryFilter")}<select value={status} className="h-10 rounded-control border border-hairline bg-canvas px-3" onChange={event => setStatus(event.target.value)}>{["", "pending", "blocked_smtp", "blocked_unverified", "failed", "accepted"].map(value => <option key={value} value={value}>{value ? t(`status_${value}`) : t("all")}</option>)}</select></label>
    {error && <p role="alert" className="mt-3 text-sm text-danger">{error}</p>}
    {query.snapshot.phase === "stale" && <p role="alert" className="mt-3 text-sm text-danger">{t("loadFailed")}</p>}
    {!data ? <p className="mt-5" role={query.snapshot.phase === "loading" ? "status" : "alert"}>{t(query.snapshot.phase === "loading" ? "loading" : "loadFailed")}</p> : !deliveries.length ? <p className="mt-5 text-sm text-ink-secondary">{t("deliveryEmpty")}</p> : <div className="mt-5 overflow-x-auto"><table className="w-full min-w-[640px] text-left text-sm"><thead><tr className="border-b border-hairline"><th className="pb-3 font-medium">{t("deliveryMessage")}</th><th className="pb-3 font-medium">{t("deliveryStatus")}</th><th className="pb-3 font-medium">{t("attempts")}</th><th className="pb-3 font-medium">{t("createdAt")}</th><th /></tr></thead><tbody>{deliveries.map(item => <tr key={item.id} className="border-b border-hairline align-top"><td className="max-w-64 py-4 pr-4"><p>{deliveryTitle(item, locale, tUI)}</p><p className="mt-1 break-all text-xs text-ink-mute">{item.brand_id}</p></td><td className="py-4 pr-4">{t.has(`status_${item.status}`) ? t(`status_${item.status}`) : item.status}{item.last_error && <p className="mt-1 text-xs text-ink-mute">{t.has(`error_${item.last_error}`) ? t(`error_${item.last_error}`) : t("actionFailed")}</p>}</td><td className="py-4 pr-4">{item.attempts}</td><td className="py-4 pr-4 text-xs text-ink-secondary">{new Date(item.created_at).toLocaleString(locale)}</td><td className="py-4">{canRetry && ["failed", "blocked_smtp", "blocked_unverified"].includes(item.status) && <Button size="sm" variant="outline" disabled={busy} onClick={() => void retry(item.id)}>{t("retry")}</Button>}</td></tr>)}</tbody></table></div>}
  </Card>;
}

export function NotificationAdminSettings() {
  return <div id="notifications" className="space-y-6 scroll-mt-24"><IfRoles roles={["platform_admin", "tech_admin"]}><SMTPSettings /></IfRoles><IfRoles roles={["platform_admin", "tech_admin", "ops_admin", "audit_readonly"]}><NotificationDeliveryLog /></IfRoles></div>;
}
