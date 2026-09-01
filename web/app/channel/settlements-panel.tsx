"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type Settlement = { id?: string; status?: string; amount_minor?: number; channel_org_id?: string };

export default function ChannelSettlements() {
  const t = useTranslations("channelUi");
  const [items, setItems] = useState<Settlement[]>([]);
  const [message, setMessage] = useState(t("settleHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/settlements`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    const next = (body.items || []) as Settlement[];
    setItems(next);
    setMessage(t("settleCount", { n: next.length }));
  }

  return (
    <Card>
      <CardTitle className="mb-3 text-xl font-medium">{t("settleTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("settleLead")}</p>
      <Button variant="outline" onClick={refresh}>
        {t("refreshSettle")}
      </Button>
      <LedgerTable
        columns={[t("colSettle"), t("colStatus"), t("colAmount")]}
        emptyTitle={t("emptySettle")}
        emptyDetail={t("emptySettleDetail")}
        rows={items.map((item) => ({
          key: item.id || "settlement",
          cells: [item.id || "—", item.status || "—", `${item.amount_minor ?? 0} micro-USD`],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
