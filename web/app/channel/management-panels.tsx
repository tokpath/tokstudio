"use client";

import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction } from "@/lib/rbac";
import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { useQuery } from "@tanstack/react-query";
import { AdminListPanel } from "@/app/admin/list-panel";
import { ConsolePageHeader } from "@/components/console/page-header";
import { LedgerTable } from "@/components/console/ledger-table";
import { BrandEditor } from "@/components/brand-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "@/components/confirm-button";
import { LocaleSwitch } from "@/components/locale-switch";
import { ThemeToggle } from "@/components/theme-toggle";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor } from "@/lib/money";
import { CommissionWorkspace } from "@/app/admin/commission/workspace";

type Channel = { id: string; code: string; type: string; parent_id?: string };
type Totals = { requests: number; revenue_minor: number; cost_minor: number; margin_minor: number; pending: number };
type Model = Totals & { model: string };

async function loadOEM<T>(path: string): Promise<T> {
  const body = await apiClient<T & { error?: { message?: string } }>("GET", path);
  if (body.error) throw new Error(body.error.message || "OEM data unavailable");
  return body;
}

export function OEMPage({ page, children }: { page: string; children: ReactNode }) {
  const ta = useTranslations("admin");
  const t = useTranslations("oem");
  return <div className="flex flex-col gap-8"><ConsolePageHeader eyebrow="OEM" title={ta(page)} description={t("scope")} />{children}</div>;
}

export function OEMScope({ children, includeAll = true }: { children: (suffix: string) => ReactNode; includeAll?: boolean }) {
  const t = useTranslations("oem");
  const viewer = useViewer();
  const [channel, setChannel] = useState("");
  useEffect(() => {
    const restore = () => setChannel(new URLSearchParams(window.location.search).get("channel_id") ?? "");
    restore(); window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, [viewer.userId]);
  const me = useQuery({ queryKey: [viewer.userId, "/channel/me"], queryFn: () => loadOEM<{ channel_org_id: string }>("/channel/me") });
  const channels = useQuery({ queryKey: [viewer.userId, "oem-scope-channels"], queryFn: async () => {
    const items: Channel[] = [];
    let cursor = "";
    do {
      const page = await loadOEM<{ items?: Channel[]; next_cursor?: string }>(`/admin/channels?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);
      items.push(...(page.items ?? []));
      if (!page.next_cursor || page.next_cursor === cursor) break;
      cursor = page.next_cursor;
    } while (true);
    return items;
  } });
  if (channels.isError || me.isError) return <p role="alert">{t("loadError")}</p>;
  if (!channels.data || !me.data) return <p role="status">{t("loading")}</p>;
  const owned = channels.data.filter((item) => item.id === me.data.channel_org_id || (item.type === "B" && item.parent_id === me.data.channel_org_id));
  const invalid = Boolean(channel && !owned.some(item => item.id === channel));
  const selected = invalid ? "" : channel || (includeAll ? "" : me.data.channel_org_id);
  function select(value: string) {
    setChannel(value);
    const url = new URL(window.location.href);
    if (value) url.searchParams.set("channel_id", value); else url.searchParams.delete("channel_id");
    window.history.replaceState(null, "", url.toString());
  }
  return <><label className="flex max-w-xs flex-col gap-2 text-sm">{t("channel")}<select aria-label={t("channel")} className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" value={selected} onChange={event => select(event.target.value)}>{includeAll ? <option value="">{t("allChannels")}</option> : null}{owned.map(item => <option key={item.id} value={item.id}>{item.code}</option>)}</select></label>{invalid ? <p role="alert">{t("invalidScope")} <Button variant="outline" onClick={() => select("")}>{t("refresh")}</Button></p> : <div key={`${viewer.userId}:${selected}`}>{children(selected ? `?channel_id=${encodeURIComponent(selected)}` : "")}</div>}</>;
}

function Report({ suffix, margin }: { suffix: string; margin: boolean }) {
  const t = useTranslations("oem");
  const viewer = useViewer();
  const query = useQuery({ queryKey: [viewer.userId, "/channel/metrics", suffix], queryFn: () => loadOEM<{ totals: Totals; items: Model[] }>(`/channel/metrics${suffix}`) });
  const totals = query.data?.totals;
  if (query.isPending) return <p role="status">{t("loading")}</p>;
  if (query.isError || !totals) return <p role="alert">{t("loadError")} <Button variant="outline" onClick={() => void query.refetch()}>{t("refresh")}</Button></p>;
  const cards = margin ? [["revenue", formatUsdMinor(totals.revenue_minor)], ["cost", formatUsdMinor(totals.cost_minor)], ["margin", formatUsdMinor(totals.margin_minor)]] : [["requests", String(totals.requests)], ["revenue", formatUsdMinor(totals.revenue_minor)], ["pending", String(totals.pending)]];
  return <><section className="grid gap-4 md:grid-cols-3">{cards.map(([key, value]) => <div key={key} className="rounded-card border border-hairline bg-canvas-raised p-5"><p className="text-sm text-ink-secondary">{t(key)}</p><p className="mt-3 text-2xl font-semibold tabular-nums">{value}</p></div>)}</section><section className="rounded-card border border-hairline bg-canvas-raised p-6"><div className="mb-4 flex items-center justify-between"><h2 className="font-semibold">{t("models")}</h2><Button variant="outline" size="sm" onClick={() => void query.refetch()}>{t("refresh")}</Button></div>{margin ? <p className="mb-4 text-sm text-ink-secondary">{t("marginHint")}</p> : null}<LedgerTable columns={[t("model"), t("requests"), t("revenue"), t("cost"), t("margin")]} rows={(query.data?.items ?? []).map((item) => ({ key: item.model, cells: [item.model, item.requests, formatUsdMinor(item.revenue_minor), formatUsdMinor(item.cost_minor), formatUsdMinor(item.margin_minor)] }))} emptyTitle={t("empty")} emptyDetail={t("emptyUsage")} /></section></>;
}

export function OEMReportPage({ margin = false }: { margin?: boolean }) {
  return <OEMPage page={margin ? "margin" : "metrics"}><OEMScope>{(suffix) => <Report suffix={suffix} margin={margin} />}</OEMScope></OEMPage>;
}

export function OEMMediaPage() {
  const t = useTranslations("oem");
  return <OEMPage page="media"><OEMScope>{(suffix) => <AdminListPanel path={`/channel/media${suffix}`} title={t("mediaTasks")} columns={[{ accessorKey: "id", header: t("id") }, { accessorKey: "kind", header: t("kind") }, { accessorKey: "model", header: t("model") }, { accessorKey: "status", header: t("status") }, { accessorKey: "progress", header: t("progress") }, { accessorKey: "created_at", header: t("time") }]} />}</OEMScope></OEMPage>;
}

export function OEMAuditPage() {
  const t = useTranslations("oem");
  return <OEMPage page="audit"><OEMScope>{(suffix) => <AdminListPanel path={`/channel/audit-logs${suffix}`} title={t("auditLog")} columns={[{ accessorKey: "actor_email", header: t("actor"), cell: ({ row }) => String(row.original.actor_email || row.original.actor_user_id || "—") }, { accessorKey: "action", header: t("action") }, { accessorKey: "resource_type", header: t("resourceType") }, { accessorKey: "resource_id", header: t("resource") }, { accessorKey: "created_at", header: t("time") }]} />}</OEMScope></OEMPage>;
}

function Alerts({ suffix }: { suffix: string }) {
  const t = useTranslations("oem");
  const viewer = useViewer();
  const query = useQuery({ queryKey: [viewer.userId, "/channel/alerts", suffix], queryFn: () => loadOEM<{ items: { id: string; title: string; count: number; href: string }[] }>(`/channel/alerts${suffix}`) });
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6"><div className="flex items-center justify-between"><h2 className="font-semibold">{t("businessAlerts")}</h2><Button variant="outline" onClick={() => void query.refetch()}>{t("refresh")}</Button></div>{query.isPending ? <p role="status" className="mt-4">{t("loading")}</p> : query.isError ? <p role="alert" className="mt-4">{t("loadError")}</p> : <ul className="mt-4 divide-y divide-hairline">{query.data?.items.map((item) => <li key={item.id} className="flex items-center justify-between py-4"><span>{item.title} · {item.count}</span><Link className="text-brand-emphasis underline" href={item.href}>{t("view")}</Link></li>)}{!query.data?.items.length ? <li className="text-sm text-ink-secondary">{t("noAlerts")}</li> : null}</ul>}<Link className="mt-5 inline-block text-sm text-brand-emphasis underline" href="/channel/runbooks">{t("runbooks")}</Link></section>;
}

export function OEMAlertsPage() {
  return <OEMPage page="alerts"><OEMScope>{(suffix) => <Alerts suffix={suffix} />}</OEMScope></OEMPage>;
}

export function OEMRunbooksPage() {
  const t = useTranslations("oem");
  return <OEMPage page="runbooks">{["models", "users", "reconciliation", "payments"].map((key) => <section key={key} className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="font-semibold">{t(`runbook.${key}.title`)}</h2><p className="mt-3 text-sm leading-6 text-ink-secondary">{t(`runbook.${key}.body`)}</p><Link className="mt-4 inline-block text-sm text-brand-emphasis underline" href={{ models: "/channel/subchannels", users: "/channel/users", reconciliation: "/channel/reconciliation", payments: "/channel/payments/orders" }[key] ?? "/channel"}>{t("view")}</Link></section>)}</OEMPage>;
}

export function OEMCommissionPage() {
  return <OEMPage page="commission"><CommissionWorkspace oem /></OEMPage>;
}

function OEMSecurity() {
  const t = useTranslations("oem");
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [message, setMessage] = useState("");
  const query = useQuery({ queryKey: ["/channel/me/2fa"], queryFn: () => loadOEM<{ item: { status: string } }>("/channel/me/2fa") });
  async function mutate(action: string) {
    setMessage("");
    try {
      const body = await apiClient<{ item?: { secret?: string }; error?: { message?: string } }>("POST", `/channel/me/2fa/${action}`, { headers: { ...confirmHeaders, ...(action === "disable" ? { "X-Tokenhub-TOTP": code } : {}) }, body: JSON.stringify({ code }) });
      if (body.error) { setMessage(body.error.message || t("saveError")); return false; }
      setSecret(action === "setup" ? body.item?.secret || "" : "");
      setCode("");
      await query.refetch();
      setMessage(t("saved"));
      return true;
    } catch (error) { setMessage(error instanceof Error && "status" in error && Number(error.status) >= 400 && Number(error.status) < 500 ? error.message : t("saveError")); return false; }
  }
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="font-semibold">{t("security")}</h2><p className="mt-3 text-sm text-ink-secondary">{t("securityHint")}</p>{query.isError ? <p role="alert">{t("loadError")}</p> : <p className="mt-3">{t("status")}: {query.data?.item.status ?? "—"}</p>}<div className="mt-4 flex max-w-xl flex-wrap gap-3">{query.data?.item.status !== "enabled" ? <Button variant="outline" disabled={!query.data} onClick={() => void mutate("setup")}>{t("setup2fa")}</Button> : null}{query.data?.item.status === "pending" || query.data?.item.status === "enabled" ? <><Input aria-label={t("otp")} placeholder={t("otp")} className="max-w-48" value={code} onChange={(e) => setCode(e.target.value)} />{query.data?.item.status === "pending" ? <Button disabled={!/^\d{6}$/.test(code)} onClick={() => void mutate("enable")}>{t("enable2fa")}</Button> : <ConfirmButton disabled={!/^\d{6}$/.test(code)} title={t("disable2fa")} description={t("disableHint")} onConfirm={() => mutate("disable")}>{t("disable2fa")}</ConfirmButton>}</> : null}</div>{secret ? <p className="mt-3 break-all font-mono text-sm">{t("secret")}: {secret}</p> : null}{message ? <p role="status" className="mt-3 text-sm">{message}</p> : null}</section>;
}

export function OEMSettingsPage() {
  const viewer = useViewer();
  const t = useTranslations("oem");
  return <OEMPage page="settings"><section className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="mb-3 font-semibold">{t("appearance")}</h2><div className="flex gap-3"><LocaleSwitch /><ThemeToggle /></div></section><OEMSecurity /><BrandEditor readOnly={!canChannelAction("operations", viewer)} endpoint="/channel/brand" uploadEndpoint="/channel/brand/assets" /></OEMPage>;
}
