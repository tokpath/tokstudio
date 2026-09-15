"use client";

import { useTranslations } from "next-intl";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { ScrollTable } from "@/components/ui/scroll-table";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";
import { formatUsageTime, shortKeyRef, usageTokens } from "@/lib/usage";

type UsageRow = {
  id: string;
  request_id?: string;
  state?: string;
  customer_amount_minor?: number;
  public_model_id?: string;
  api_key_id?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  occurred_at?: string;
};

function formatAmountUsd(minor?: number) {
  if (typeof minor !== "number" || !Number.isFinite(minor)) {
    return "—";
  }
  return `$${(minor / 1_000_000).toFixed(2)}`;
}

/** 请求明细：宽表钉首末列，对齐 DESIGN.md §4.5 / docs/14。 */
export function ActivityTable() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const list = useListResource<UsageRow>({
    load: () => fetchListItems(`${apiBase}/v1/me/usage`),
  });

  return (
    <div className="flex flex-col gap-3">
      <Button variant="outline" size="sm" className="self-start" onClick={() => void list.reload()}>
        {tc("refresh")}
      </Button>
      <ListResourceView
        snapshot={list.snapshot}
        loadingTitle={t("actLoading")}
        emptyTitle={t("actEmpty")}
        emptyDetail={t("actEmptyDetail")}
        onRetry={() => void list.reload()}
      >
        <ScrollTable
          density="ledger"
          className="rounded-card border border-hairline"
          getRowId={(row) => row.id}
          rows={list.snapshot.items}
          columns={[
            {
              id: "time",
              header: t("colTime"),
              cell: (row) => <span className="text-xs text-ink-mute">{formatUsageTime(row.occurred_at)}</span>,
            },
            {
              id: "key",
              header: t("colApiKey"),
              cell: (row) => <span className="font-mono text-xs">{shortKeyRef(row.api_key_id)}</span>,
            },
            {
              id: "model",
              header: t("colModel"),
              cell: (row) => <span className="font-mono text-xs">{row.public_model_id || "—"}</span>,
            },
            {
              id: "prompt",
              header: t("colPrompt"),
              cell: (row) => <span className="font-mono tabular-nums">{usageTokens(row).prompt}</span>,
            },
            {
              id: "completion",
              header: t("colCompletion"),
              cell: (row) => <span className="font-mono tabular-nums">{usageTokens(row).completion}</span>,
            },
            {
              id: "status",
              header: t("colStatus"),
              cell: (row) => row.state || "—",
            },
            {
              id: "req",
              header: t("colReq"),
              cell: (row) => <span className="font-mono text-xs text-ink-mute">{row.request_id || row.id}</span>,
            },
            {
              id: "amount",
              header: t("colAmount"),
              cell: (row) => <span className="font-mono tabular-nums">{formatAmountUsd(row.customer_amount_minor)}</span>,
            },
          ]}
        />
      </ListResourceView>
    </div>
  );
}
