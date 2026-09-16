"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ScrollTable } from "@/components/ui/scroll-table";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";
import { statusLabelKey, statusTone } from "@/lib/status-copy";
import { copyText } from "@/lib/submit-result";
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

function useStatusText() {
  const tc = useTranslations("common");
  return (status?: string) => {
    const key = statusLabelKey(status);
    if (key) {
      return tc(key);
    }
    if (!status) {
      return "—";
    }
    return tc("stUnknown", { status });
  };
}

/** 请求明细：宽表钉首末列，对齐 DESIGN.md §4.5 / docs/14。 */
export function ActivityTable() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const statusText = useStatusText();
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
              cell: (row) => <Badge tone={statusTone(row.state)}>{statusText(row.state)}</Badge>,
            },
            {
              id: "amount",
              header: t("colAmount"),
              cell: (row) => (
                <span className="font-mono tabular-nums">{formatUsdMinor(row.customer_amount_minor, tc("lessThanCent"))}</span>
              ),
            },
            {
              id: "details",
              header: tc("details"),
              cell: (row) => <RequestDetails id={row.request_id || row.id} />,
            },
          ]}
        />
      </ListResourceView>
    </div>
  );
}

function RequestDetails({ id }: { id: string }) {
  const tc = useTranslations("common");
  const [copied, setCopied] = useState(false);
  return (
    <details className="text-xs">
      <summary className="cursor-pointer text-ink-secondary">{tc("details")}</summary>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <span className="text-ink-mute">{tc("requestNo")}</span>
        <span className="font-mono text-ink">{id}</span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          aria-label={tc("copyRequest")}
          onClick={() => {
            void copyText(id).then((ok) => setCopied(ok));
          }}
        >
          {copied ? tc("copied") : tc("copy")}
        </Button>
      </div>
    </details>
  );
}
