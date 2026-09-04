"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type ChannelKey = {
  id?: string;
  user_email?: string;
  name?: string;
  prefix?: string;
  status?: string;
  last_used_at?: string | null;
};

function formatLastUsed(value?: string | null) {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export default function ChannelKeys() {
  const t = useTranslations("channelUi");
  const [items, setItems] = useState<ChannelKey[]>([]);
  const [message, setMessage] = useState(t("keysHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/api-keys`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    const next = (body.items || []) as ChannelKey[];
    setItems(next);
    setMessage(t("keysCount", { n: next.length }));
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("keysTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("keysLead")}</p>
      <Button variant="outline" onClick={refresh}>
        {t("refreshKeys")}
      </Button>
      <LedgerTable
        columns={[t("colEmail"), t("colKeyName"), t("colPrefix"), t("colStatus"), t("colLastUsed"), t("disableKey")]}
        emptyTitle={t("emptyKeys")}
        emptyDetail={t("emptyKeysDetail")}
        rows={items.map((item) => ({
          key: item.id || `${item.prefix}-${item.name}`,
          cells: [
            item.user_email || "—",
            item.name || "—",
            item.prefix || "—",
            item.status || "—",
            formatLastUsed(item.last_used_at),
            item.id && item.status !== "disabled" ? (
              <ConfirmButton
                size="sm"
                variant="outline"
                title={t("confirmDisableKey")}
                description={t("confirmDisableKeyD")}
                onConfirm={async () => {
                  const response = await fetch(`${apiBase}/channel/api-keys/${item.id}/disable`, {
                    method: "POST",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: "{}",
                  });
                  const body = await response.json();
                  if (!response.ok) {
                    setMessage(body.error?.message || t("needAdmin"));
                    return;
                  }
                  setMessage(t("disabledKey", { id: body.item?.id || item.id, status: body.item?.status || "disabled" }));
                  await refresh();
                }}
              >
                {t("disableKey")}
              </ConfirmButton>
            ) : (
              "—"
            ),
          ],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
