"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";

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
  const [rows, setRows] = useState<LedgerRow[] | null>(null);
  const [message, setMessage] = useState(t("ledHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/ledger`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setRows([]);
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    setRows(body.items || []);
    setMessage(t("ledDone"));
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <section>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{t("ledTitle")}</h3>
        <Button variant="outline" size="sm" onClick={() => void refresh()}>
          {tc("refresh")}
        </Button>
      </div>
      {rows === null ? (
        <EmptyLedger title={t("ledLoading")} detail={t("ledHint")} />
      ) : (
        <LedgerTable
          columns={[t("ledColType"), t("ledColAmount")]}
          emptyTitle={t("ledEmpty")}
          emptyDetail={message}
          rows={rows.map((row) => ({
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
      )}
    </section>
  );
}
