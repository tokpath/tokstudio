"use client";

import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";

type Settlement = { id?: string; status?: string; amount_minor?: number; channel_org_id?: string };

export default function ChannelSettlements() {
  const t = useTranslations("channelUi");
  const list = useListResource<Settlement>({
    load: () => fetchListItems(`${apiBase}/channel/settlements`),
  });

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("settleTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("settleLead")}</p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {t("refreshSettle")}
      </Button>
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={t("emptySettle")}
        emptyDetail={t("emptySettleDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colSettle"), t("colStatus"), t("colAmount")]}
          emptyTitle={t("emptySettle")}
          emptyDetail={t("emptySettleDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: item.id || "settlement",
            cells: [item.id || "—", item.status || "—", formatUsdMinor(item.amount_minor)],
          }))}
        />
      </ListResourceView>
    </Card>
  );
}
