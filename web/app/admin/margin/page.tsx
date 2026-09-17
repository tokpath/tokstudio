"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { AdminH2 } from "@/components/admin-h2";
import { SealConfirm } from "@/components/seal-confirm";
import { IfCan } from "@/components/rbac/if-can";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { UpstreamFactsBadge } from "@/components/upstream-facts-badge";
import {
  COST_SOURCE,
  emptyAttemptCostTitle,
  marginClassName,
  type AttemptCostRow,
  type MarginView,
} from "@/lib/margin";
import { fromDiffRow } from "@/lib/upstream-facts";

function micro(n: unknown) {
  const v = Number(n);
  if (!Number.isFinite(v)) return "—";
  return `$${(v / 1_000_000).toFixed(6)}`;
}

export default function AdminMarginPage() {
  const t = useTranslations("adminUi");
  const [selected, setSelected] = useState<AttemptCostRow | null>(null);
  const [message, setMessage] = useState(t("marginLead"));

  const query = useQuery({
    queryKey: ["admin-margin"],
    queryFn: () => apiClient<{ item?: MarginView; items?: AttemptCostRow[] }>("GET", "/admin/margin"),
  });
  const summary = query.data?.item || {};
  const items = useMemo(() => query.data?.items || summary.items || [], [query.data, summary.items]);

  async function fileCorrection(kind: "fill_cost" | "adjust_margin"): Promise<boolean> {
    try {
    const requestID = selected?.request_id;
    if (!requestID) {
      setMessage(t("correctionNeedRow"));
      return false;
    }
    const res = await fetch(`${apiBase}/admin/margin/corrections`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        kind,
        request_id: requestID,
        attempt_id: selected?.attempt_id,
        idempotency_key: `mcr-${kind}-${requestID}-${Date.now()}`,
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("correctionFiled", { id: body.item?.id || "" }) : body.error?.message || t("marginLead"));
    const __ok = res.ok;
    if (res.ok) await query.refetch();
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  const stats = [
    { k: t("marginCost"), v: micro(summary.attempt_cost_minor) },
    { k: t("marginSell"), v: micro(summary.sell_minor) },
    { k: t("marginProfit"), v: micro(summary.margin_minor), danger: Number(summary.margin_minor) < 0 },
  ];

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="margin" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">{t("marginLead")}</p>
        <dl aria-label={t("margin")} className="mb-4 grid gap-3 sm:grid-cols-3">
          {stats.map((item) => (
            <div key={item.k} className="border-b border-hairline py-2">
              <dt className="text-sm text-ink-secondary">{item.k}</dt>
              <dd
                className={`text-right font-mono text-lg tabular-nums ${item.danger ? marginClassName(-1) : ""}`}
              >
                {item.v}
              </dd>
            </div>
          ))}
        </dl>
        <div className="mb-3 flex flex-wrap gap-2">
          <IfCan action="margin.correct">
            <SealConfirm
              size="sm"
              title={t("fillCostTitle")}
              description={t("fillCostDesc")}
              validate={() => Boolean(selected?.request_id)}
              onConfirm={() => fileCorrection("fill_cost")}
            >
              {t("fillCost")}
            </SealConfirm>
            <SealConfirm
              size="sm"
              variant="outline"
              title={t("adjustMarginTitle")}
              description={t("adjustMarginDesc")}
              validate={() => Boolean(selected?.request_id)}
              onConfirm={() => fileCorrection("adjust_margin")}
            >
              {t("adjustMargin")}
            </SealConfirm>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
        <p className="mt-2 text-sm text-ink-mute">
          <Link href="/admin/reconciliation" className="underline-offset-2 hover:underline">
            {t("toPendingQueue")}
          </Link>
        </p>
      </section>
      <AdminListPanel<AttemptCostRow>
        path="/admin/margin"
        title={t("margin")}
        density="ledger"
        emptyTitle={emptyAttemptCostTitle()}
        emptyDetail={t("marginEmptyDetail")}
        onRowSelect={(row) => setSelected(row)}
        rowSelected={(row) => row.attempt_id === selected?.attempt_id && row.request_id === selected?.request_id}
        columns={[
          { accessorKey: "attempt_id", header: t("colAttempt") },
          {
            id: "upstream_facts",
            header: t("colUpstreamFacts"),
            cell: ({ row }) => <UpstreamFactsBadge facts={fromDiffRow(row.original)} />,
          },
          {
            id: "source",
            header: t("marginSource"),
            cell: () => <span className="font-mono">{COST_SOURCE}</span>,
          },
          {
            id: "upstream",
            header: t("colUpstream"),
            cell: ({ row }) => <span className="font-mono tabular-nums">{row.original.prices?.upstream || "—"}</span>,
          },
          {
            id: "wholesale",
            header: t("colWholesale"),
            cell: ({ row }) => <span className="font-mono tabular-nums">{row.original.prices?.wholesale || "—"}</span>,
          },
          {
            id: "sell_price",
            header: t("colSell"),
            cell: ({ row }) => <span className="font-mono tabular-nums">{row.original.prices?.sell || "—"}</span>,
          },
          {
            id: "channel_price",
            header: t("colChannelPrice"),
            cell: ({ row }) => <span className="font-mono tabular-nums">{row.original.prices?.channel || "—"}</span>,
          },
          {
            id: "cost",
            header: t("colCost"),
            cell: ({ row }) => <span className="font-mono tabular-nums">{micro(row.original.cost_minor)}</span>,
          },
          {
            id: "sell_amt",
            header: t("colSell"),
            cell: ({ row }) => <span className="font-mono tabular-nums">{micro(row.original.sell_minor)}</span>,
          },
          {
            id: "margin",
            header: t("colMargin"),
            cell: ({ row }) => (
              <span
                data-testid={Number(row.original.margin_minor) < 0 ? "margin-danger" : "margin-ok"}
                className={`font-mono tabular-nums ${marginClassName(Number(row.original.margin_minor) || 0)}`}
              >
                {micro(row.original.margin_minor)}
              </span>
            ),
          },
        ]}
      />
      {items.length === 0 ? null : (
        <p className="sr-only">{t("marginSource")}: {COST_SOURCE}</p>
      )}
    </AdminShell>
  );
}
