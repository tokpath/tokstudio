"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type Bucket = {
  source_code?: string;
  role_type?: string;
  user_count?: number;
};

export default function ChannelAttribution() {
  const t = useTranslations("channelUi");
  const [items, setItems] = useState<Bucket[]>([]);
  const [message, setMessage] = useState(t("attrHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/attribution`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    const next = (body.items || []) as Bucket[];
    setItems(next);
    setMessage(t("attrCount", { n: next.length }));
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("attrTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("attrLead")}</p>
      <Button variant="outline" onClick={refresh}>
        {t("refreshAttr")}
      </Button>
      <LedgerTable
        columns={[t("colPromo"), t("colLevel"), t("colPeople")]}
        emptyTitle={t("emptyAttr")}
        emptyDetail={t("emptyAttrDetail")}
        rows={items.map((item) => ({
          key: `${item.source_code}-${item.role_type}`,
          cells: [item.source_code || "—", item.role_type || t("noLevel"), String(item.user_count ?? 0)],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
