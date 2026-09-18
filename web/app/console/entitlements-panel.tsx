"use client";

import { useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import { formatUsdMinor } from "@/lib/money";
import { Button } from "@/components/ui/button";

type Entitlement = { id: string; source_type: string; unit_type: string; granted: number; remaining: number; status: string; expires_at?: string; public_model_id?: string };

export function EntitlementsPanel() {
  const t = useTranslations("entitlements");
  const locale = useLocale();
  const [items, setItems] = useState<Entitlement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  async function refresh() {
    setLoading(true); setError("");
    try {
      const response = await fetch(`${apiBase}/v1/me/entitlements`, { credentials: "include" });
      const body = await response.json();
      if (!response.ok || !Array.isArray(body.items)) throw new Error();
      setItems(body.items);
    } catch { setError(t("loadError")); }
    finally { setLoading(false); }
  }
  useEffect(() => { void refresh(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  function quantity(n: number, unit: string) {
    return unit === "usd_credit" ? `${formatUsdMinor(n)} USD` : `${n.toLocaleString(locale)} ${["token", "video_second", "image_count"].includes(unit) ? t(unit) : unit}`;
  }
  return <section className="mt-6 space-y-3 border-t border-hairline pt-5" aria-label={t("title")}>
    <div className="flex items-center justify-between gap-3"><h3 className="font-semibold">{t("title")}</h3><Button type="button" size="sm" variant="outline" disabled={loading} onClick={() => void refresh()}>{t("refresh")}</Button></div>
    <p className="text-sm text-ink-secondary">{t("help")}</p>
    {loading ? <p role="status">{t("loading")}</p> : error ? <p role="alert" className="text-danger">{error}</p> : !items.length ? <p>{t("empty")}</p> : <ul className="space-y-3">{items.map(item => {
      const expired = !!item.expires_at && Date.parse(item.expires_at) <= Date.now();
      const status = item.status === "active" ? (expired ? "expired" : item.remaining <= 0 ? "exhausted" : "active") : item.status;
      return <li key={item.id} className="rounded-control border border-hairline p-3 text-sm">
        <p className="font-medium">{item.source_type === "bonus" ? t("bonus") : t("plan")} · {t.has(status) ? t(status) : item.status}</p>
        <p>{t("remaining", { amount: quantity(status === "active" ? item.remaining : 0, item.unit_type) })}</p>
        <p className="text-ink-secondary">{t("granted", { amount: quantity(item.granted, item.unit_type) })}</p>
        <p>{t("expiry", { time: item.expires_at ? new Date(item.expires_at).toLocaleString(locale, { timeZoneName: "short" }) : t("noExpiry") })}</p>
        {item.public_model_id ? <p>{t("model", { model: item.public_model_id })}</p> : null}
      </li>;
    })}</ul>}
  </section>;
}
