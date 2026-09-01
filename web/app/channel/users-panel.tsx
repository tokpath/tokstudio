"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type ChannelUser = { id?: string; email?: string; status?: string; source_code?: string };

export default function ChannelUsers() {
  const t = useTranslations("channelUi");
  const [items, setItems] = useState<ChannelUser[]>([]);
  const [message, setMessage] = useState(t("usersHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/users`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    const next = (body.items || []) as ChannelUser[];
    setItems(next);
    setMessage(t("usersCount", { n: next.length }));
  }

  return (
    <Card className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <CardTitle className="mb-3 text-xl font-medium">{t("usersTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("usersLead")}</p>
      <Button variant="outline" onClick={refresh}>
        {t("refreshUsers")}
      </Button>
      <LedgerTable
        columns={[t("colEmail"), t("colStatus"), t("colCode")]}
        emptyTitle={t("emptyUsers")}
        emptyDetail={t("emptyUsersDetail")}
        rows={items.map((item) => ({
          key: item.id || `${item.email}-${item.source_code}`,
          cells: [item.email || "—", item.status || "—", item.source_code || "—"],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
