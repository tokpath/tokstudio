"use client";

import { useLocale, useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";

type LedgerRow = {
  id: string;
  event_type?: string;
  created_at?: string;
  amount_minor?: number;
};

export function WalletLedger() {
  const t = useTranslations("user");
  const locale = useLocale();
  const events = new Set(["topup", "authorization", "release", "usage_debit", "refund", "adjustment", "commission_debit", "commission_payout", "gift_credit", "commission_credit"]);
  const tc = useTranslations("common");
  const list = useListResource<LedgerRow>({
    load: () => fetchListItems(`${apiBase}/v1/me/ledger`),
  });

  return (
    <section>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{t("ledTitle")}</h3>
        <Button variant="outline" size="sm" onClick={() => void list.reload()}>
          {tc("refresh")}
        </Button>
      </div>
      <ListResourceView
        name="ledger"
        snapshot={list.snapshot}
        loadingTitle={t("ledLoading")}
        emptyTitle={t("ledEmpty")}
        emptyDetail={t("ledEmptyDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("ledColType"), t("ledColAmount"), t("ledColDate")]}
          emptyTitle={t("ledEmpty")}
          emptyDetail={t("ledEmptyDetail")}
          rows={list.snapshot.items.map((row) => ({
            key: row.id,
            cells: [
              <span key="type" className="font-mono text-ink-mute">
                {t(`ledgerEvents.${events.has(row.event_type || "") ? row.event_type : "other"}`)}
              </span>,
              <span key="amount" className="font-mono tabular-nums">
                {formatUsdMinor(row.amount_minor)}
              </span>,
              row.created_at ? new Date(row.created_at).toLocaleString(locale) : "—",
            ],
          }))}
        />
      </ListResourceView>
    </section>
  );
}
