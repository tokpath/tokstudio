"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { canViewAdminHref } from "@/lib/rbac";
import { perTokenToPerMillion } from "@/lib/token-price";
import { type AdminModel } from "@/lib/catalog";

export function ModelServicePanel({ model }: { model: AdminModel }) {
  const t = useTranslations("modelService");
  const viewer = useViewer();
  const state = model.service_readiness;
  const routeHref = state?.route_ids[0] ? `/admin/routes/${encodeURIComponent(state.route_ids[0])}` : `/admin/routes/new?model=${encodeURIComponent(model.id)}`;
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6">
    <h2 className="text-lg font-semibold">{model.display_name}</h2>
    <p className="mt-2 break-all text-sm">{model.vendor} · {model.kind || String(model.capabilities?.kind || "—")} · <code>{model.id}</code></p>
    <p className="mt-3 font-medium">{t(`state.${state?.runtime_state || "unknown"}`)}</p>
    <p className="mt-1 text-sm text-ink-secondary">{t("healthHint")}</p>
    {state?.missing.length ? <ul className="my-3 list-inside list-disc text-sm">{state.missing.map(item => <li key={item}>{t(`missing.${item}`)}</li>)}</ul> : null}
    <div className="mt-4 flex flex-wrap gap-4 text-sm">
      {canViewAdminHref(routeHref, viewer) ? <Link className="text-brand-emphasis underline" href={routeHref}>{state?.route_ids.length ? t("existingRoute") : t("newRoute")}</Link> : null}
      {canViewAdminHref("/admin/channels", viewer) ? <Link className="text-brand-emphasis underline" href={`/admin/channels?model=${encodeURIComponent(model.id)}`}>{t("authorize")}</Link> : null}
    </div>
    {state?.providers.length ? <div className="mt-5 grid gap-3 sm:grid-cols-2">{state.providers.map(provider => <div key={`${provider.route_id}:${provider.provider_id}`} className="rounded-control border border-hairline p-3 text-sm">
      {canViewAdminHref("/admin/providers", viewer) ? <Link className="break-all text-brand-emphasis underline" href={`/admin/providers/${encodeURIComponent(provider.provider_id)}?model=${encodeURIComponent(model.id)}`}>{provider.provider_id}</Link> : <span>{provider.provider_id}</span>}
      <p className="my-1">{provider.upstream_model_id || "—"} · {t(`state.${provider.runtime_state}`)}</p>
      {provider.missing.map(item => <p key={item}>{t(`missing.${item}`)}</p>)}
      {provider.checked_at ? <p className="text-ink-secondary">{t("checkedAt", { time: new Date(provider.checked_at).toLocaleString(undefined, { timeZone: "Asia/Shanghai" }) + " (UTC+8)" })}</p> : null}
    </div>)}</div> : null}
    <details className="mt-5 text-sm"><summary className="cursor-pointer">{t("price")}</summary><dl className="mt-3 grid gap-2">{Object.entries(model.sell_price || {}).map(([key,value]) => <div key={key} className="flex gap-3"><dt>{["input", "output", "image_count", "video_second", "audio_second"].includes(key) ? t(key) : key}</dt><dd className="break-all">{["input", "output"].includes(key) ? perTokenToPerMillion(String(value)) : typeof value === "object" ? JSON.stringify(value) : String(value)}</dd></div>)}</dl></details>
  </section>;
}
