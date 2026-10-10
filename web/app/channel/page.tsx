"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { apiClient } from "@/lib/client";
import { formatUsdMinor } from "@/lib/money";
import { canViewChannelHref } from "@/lib/rbac";
import { useViewer } from "@/components/rbac/viewer-context";
import { Button } from "@/components/ui/button";

type Home = { channel_name: string; brand_name: string };
type Delivery = { ready: boolean; checks: { ready: boolean }[]; delivery: { phase: string } };
export default function ChannelConsole() {
  const viewer = useViewer();
  const t = useTranslations("channelWorkbench");
  const oem = viewer.channelType === "C";
  const scope = [viewer.userId, viewer.roles.join(","), viewer.channelType];
  const me = useQuery({queryKey: ["channel-workbench", ...scope], queryFn: () => apiClient<Home>("GET", "/channel/me")});
  const delivery = useQuery({queryKey: ["channel-workbench-delivery", ...scope], enabled: oem, queryFn: () => apiClient<{item: Delivery}>("GET", "/channel/delivery")});
  const personal = useQuery({queryKey: ["channel-workbench-income", ...scope], enabled: !oem, queryFn: () => apiClient<{item: {invited_count: number; summary: {frozen_minor: number; available_minor: number; paid_minor: number}}}>("GET", "/v1/me/referral")});
  const tasks = oem ? [
    {key:"delivery", href:"/channel/delivery"}, {key:"customers", href:"/channel/users"},
    {key:"payments", href:"/channel/payments"}, {key:"catalog", href:"/channel/models"},
    {key:"commission", href:"/channel/commission"}, {key:"exceptions", href:"/channel/reconciliation"},
    {key:"brand", href:"/channel/brand"},
  ] : [{key:"customers", href:"/channel/users"}, {key:"promote", href:"/channel/promos"}, {key:"models", href:"/channel/models"}, {key:"earnings", href:"/channel/commissions"}];
  const failed = me.isError || (oem ? delivery.isError : personal.isError);
  const loading = me.isPending || (oem ? delivery.isPending : personal.isPending);
  return <div className="space-y-6">
    <header><h1 className="text-2xl font-semibold">{t(oem ? "oemTitle" : "channelTitle")}</h1>{me.data && <p className="mt-2 text-ink-secondary">{me.data.brand_name} · {me.data.channel_name}</p>}</header>
    {loading && <p role="status">{t("loading")}</p>}
    {failed && <div role="alert" className="rounded-control border border-danger/30 p-4"><p>{t("readFailed")}</p><Button variant="outline" className="mt-2" onClick={() => {void me.refetch(); if(oem)void delivery.refetch();else void personal.refetch();}}>{t("retry")}</Button></div>}
    {oem && delivery.data && <section className="rounded-card border border-hairline p-5"><h2 className="font-semibold">{t("delivery")}</h2><p className="my-3">{delivery.data.item.ready ? t("ready") : t("missing", {count: delivery.data.item.checks.filter(check => !check.ready).length})}</p><Button asChild variant="outline"><Link href="/channel/delivery">{t("reviewDelivery")}</Link></Button></section>}
    {!oem && personal.data && <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4" aria-label={t("earnings")}>
      {[{key:"invited", value:String(personal.data.item.invited_count)}, {key:"frozen", value:formatUsdMinor(personal.data.item.summary.frozen_minor)}, {key:"available", value:formatUsdMinor(personal.data.item.summary.available_minor)}, {key:"paid", value:formatUsdMinor(personal.data.item.summary.paid_minor)}].map(item => <div key={item.key} className="rounded-card border border-hairline p-5"><p className="text-sm text-ink-secondary">{t(item.key)}</p><p className="mt-2 text-xl font-medium tabular-nums">{item.value}</p></div>)}
    </section>}
    <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-label={t("tasks")}>{tasks.filter(item => canViewChannelHref(item.href, viewer)).map(item => <Link key={item.key} href={item.href} className="rounded-card border border-hairline bg-canvas-raised p-5 text-ink no-underline hover:border-brand"><h2 className="font-semibold">{t(item.key)}</h2><p className="mt-2 text-sm text-ink-secondary">{t(`${item.key}Hint`)}</p><p className="mt-4 text-sm text-brand-emphasis">{t("open")} →</p></Link>)}</section>
  </div>;
}
