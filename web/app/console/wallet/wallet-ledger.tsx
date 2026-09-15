"use client";

import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";

type LedgerRow = {
  id: string;
  entry_type?: string;
  amount_minor?: number;
};

function formatMinor(amount?: number) {
  if (amount == null) return "—";
  return `$${(amount / 1_000_000).toFixed(2)}`;
}

export function WalletLedger() {
  const t = useTranslations("user");
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
          columns={[t("ledColType"), t("ledColAmount")]}
          emptyTitle={t("ledEmpty")}
          emptyDetail={t("ledEmptyDetail")}
          rows={list.snapshot.items.map((row) => ({
            key: row.id,
            cells: [
              <span key="type" className="font-mono text-ink-mute">
                {row.entry_type || row.id}
              </span>,
              <span key="amount" className="font-mono tabular-nums">
                {formatMinor(row.amount_minor)}
              </span>,
            ],
          }))}
        />
      </ListResourceView>
    </section>
  );
}
