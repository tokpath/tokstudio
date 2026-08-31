"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";

type LedgerRow = {
  id: string;
  entry_type?: string;
  amount_minor?: number;
};

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
        <h2 className="text-lg font-semibold">{t("ledTitle")}</h2>
        <Button variant="outline" size="sm" onClick={() => void refresh()}>
          {tc("refresh")}
        </Button>
      </div>
      {!rows || rows.length === 0 ? (
        <EmptyLedger title={t("ledEmpty")} detail={message} />
      ) : (
        <ul className="divide-y divide-hairline rounded-card border border-hairline bg-canvas-raised">
          {rows.map((row) => (
            <li key={row.id} className="flex items-center justify-between gap-3 px-4 py-3 text-sm">
              <span className="font-mono text-ink-mute">{row.entry_type || row.id}</span>
              <span className="font-mono tabular-nums">{row.amount_minor ?? "—"}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
