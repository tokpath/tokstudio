"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";

type Allocation = {
  id?: string;
  user_id?: string;
  granted_minor?: number;
  consumed_minor?: number;
  remaining_minor?: number;
  status?: string;
};

type Quota = {
  available_minor?: string | number;
  issued_minor?: string | number;
  consumed_minor?: string | number;
  issue_ratio_bps?: string | number;
};

export default function ChannelCommissions() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [quota, setQuota] = useState<Quota>({});
  const list = useListResource<Allocation>({
    load: async () => {
      const [q, a] = await Promise.all([
        fetch(`${apiBase}/channel/quota`, { credentials: "include" }),
        fetchListItems<Allocation>(`${apiBase}/channel/allocations`),
      ]);
      let extras: { quota?: Quota } = {};
      try {
        const qBody = await q.json();
        if (q.ok) {
          extras = { quota: (qBody.quota || {}) as Quota };
        }
      } catch {
        /* quota is supplementary; allocations drive the list state */
      }
      if (!q.ok && !a.ok) {
        return {
          ok: false,
          status: q.status || a.status,
          items: [],
          message: a.message,
          code: a.code,
          network: a.network,
          extras,
        };
      }
      return { ...a, extras };
    },
    onAccepted: (result) => {
      const quota = (result.extras as { quota?: Quota } | undefined)?.quota;
      if (quota) {
        setQuota(quota);
      }
    },
  });

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <h2 className="mb-4 text-lg font-semibold tracking-tight">{t("commTitle")}</h2>
      <p className="mb-3 text-sm text-ink-secondary">{t("commLead", { quota: formatUsdMinor(quota.available_minor) })}</p>
      <p className="mb-3 text-sm text-ink-secondary">
        {t("commMeta", {
          ratio: quota.issue_ratio_bps ?? "—",
          issued: formatUsdMinor(quota.issued_minor),
          consumed: formatUsdMinor(quota.consumed_minor),
        })}
      </p>
      <h3 className="mb-2 text-lg font-medium">{t("issuedTitle")}</h3>
      <Button type="button" variant="outline" className="mb-3" onClick={() => void list.reload()}>
        {tc("refresh")}
      </Button>
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={t("emptyIssued")}
        emptyDetail={t("emptyIssuedDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colUser"), t("colGranted"), t("colUsed"), t("colLeft"), t("colStatus")]}
          emptyTitle={t("emptyIssued")}
          emptyDetail={t("emptyIssuedDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: item.id || item.user_id || "alloc",
            cells: [
              item.user_id || "—",
              String(item.granted_minor ?? 0),
              String(item.consumed_minor ?? 0),
              String(item.remaining_minor ?? 0),
              item.status || "—",
            ],
          }))}
        />
      </ListResourceView>
    </section>
  );
}
