"use client";

import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";

type ChannelUser = { id?: string; email?: string; status?: string; source_code?: string };

export default function ChannelUsers() {
  const t = useTranslations("channelUi");
  const list = useListResource<ChannelUser>({
    load: () => fetchListItems(`${apiBase}/channel/users`),
  });

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("usersTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("usersLead")}</p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {t("refreshUsers")}
      </Button>
      <ListResourceView
        snapshot={list.snapshot}
        loadingTitle={t("usersTitle")}
        emptyTitle={t("emptyUsers")}
        emptyDetail={t("emptyUsersDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colEmail"), t("colStatus"), t("colCode")]}
          emptyTitle={t("emptyUsers")}
          emptyDetail={t("emptyUsersDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: item.id || `${item.email}-${item.source_code}`,
            cells: [item.email || "—", item.status || "—", item.source_code || "—"],
          }))}
        />
      </ListResourceView>
    </Card>
  );
}
