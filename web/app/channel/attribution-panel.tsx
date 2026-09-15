"use client";

import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";

type Bucket = {
  source_code?: string;
  role_type?: string;
  user_count?: number;
};

export default function ChannelAttribution() {
  const t = useTranslations("channelUi");
  const list = useListResource<Bucket>({
    load: () => fetchListItems(`${apiBase}/channel/attribution`),
  });

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("attrTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("attrLead")}</p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {t("refreshAttr")}
      </Button>
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={t("emptyAttr")}
        emptyDetail={t("emptyAttrDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colPromo"), t("colLevel"), t("colPeople")]}
          emptyTitle={t("emptyAttr")}
          emptyDetail={t("emptyAttrDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: `${item.source_code}-${item.role_type}`,
            cells: [item.source_code || "—", item.role_type || t("noLevel"), String(item.user_count ?? 0)],
          }))}
        />
      </ListResourceView>
    </Card>
  );
}
