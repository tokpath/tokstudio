"use client";

import { useLocale, useTranslations } from "next-intl";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";

type Recovery = { id: string; settlement_id: string; amount_minor: number; recovered_minor: number; status: string; receipts: { id: string; amount_minor: number; reference: string; created_at: string }[] };
export function RecoveryHistory() {
  const t = useTranslations("recoveryHistory");
  const tc = useTranslations("common");
  const locale = useLocale();
  const list = useListResource<Recovery>({ load: () => fetchListItems(`${apiBase}/v1/me/commission-recoveries`) });
  const money = (n: number) => `${n / 1_000_000} USD`;
  return <section aria-label={t("title")}>
    <div className="mb-3 flex items-center justify-between gap-3"><h3 className="text-sm font-medium">{t("title")}</h3><Button size="sm" variant="outline" onClick={() => void list.reload()}>{tc("refresh")}</Button></div>
    <p className="mb-3 text-sm text-ink-secondary">{t("detail")}</p>
    <ListResourceView name="commission-recoveries" snapshot={list.snapshot} loadingTitle={t("loading")} emptyTitle={t("empty")} emptyDetail={t("emptyDetail")} onRetry={() => void list.reload()}>
      <ul className="grid gap-3">{list.snapshot.items.map(item => <li key={item.id} className="rounded-control border border-hairline p-4 text-sm">
        <p className="font-medium">{t(item.status === "closed" ? "closed" : "pending")}</p>
        <p>{t("amounts", { original: money(item.amount_minor), received: money(item.recovered_minor), remaining: money(item.amount_minor - item.recovered_minor) })}</p>
        <p className="break-all text-ink-secondary">{t("settlement", { id: item.settlement_id })}</p>
        <ul className="mt-2 space-y-2">{item.receipts.map(receipt => <li key={receipt.id} className="break-all">{t("receipt", { amount: money(receipt.amount_minor), reference: receipt.reference, date: new Date(receipt.created_at).toLocaleString(locale) })}</li>)}</ul>
      </li>)}</ul>
    </ListResourceView>
  </section>;
}
