"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { SealConfirm } from "@/components/seal-confirm";
import { Button } from "@/components/ui/button";
import { ScrollTable } from "@/components/ui/scroll-table";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { UpstreamFactsBadge } from "@/components/upstream-facts-badge";
import {
  type DiffRow,
  type ReconcileView,
  diffKey,
  emptyUsageTitle,
  flagPath,
  isMatchRow,
  listPath,
  pendingQueueTitle,
  reconcileActions,
  rowClassName,
  tabularMinor,
} from "@/lib/bucket-reconcile";
import { fromDiffRow } from "@/lib/upstream-facts";

type Scope = "user" | "channel";

export function BucketReconcilePanel({
  scope,
  initial,
}: {
  scope: Scope;
  initial?: ReconcileView;
}) {
  const t = useTranslations("reconcile");
  const [view, setView] = useState<ReconcileView>(initial ?? { items: [], pending: [] });
  const [message, setMessage] = useState(t("lead"));
  const [loaded, setLoaded] = useState(Boolean(initial));

  async function refresh() {
    const res = await fetch(`${apiBase}${listPath(scope)}`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || t("needLogin"));
      setLoaded(true);
      return;
    }
    setView((body.item || {}) as ReconcileView);
    setMessage(t("done"));
    setLoaded(true);
  }

  async function flag(row: DiffRow): Promise<boolean> {
    try {
    const key = diffKey(row);
    if (!key) {
      setMessage(t("needRow"));
      return false;
    }
    const res = await fetch(`${apiBase}${flagPath(scope)}`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ id: row.usage_id, request_id: row.request_id }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("flagged") : body.error?.message || t("flagFail"));
    const __ok = res.ok;
    if (res.ok) {
      await refresh();
    }
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  useEffect(() => {
    if (initial) {
      return;
    }
    void refresh();
    // 进入页面拉一次真实对账窗口。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scope]);

  const items = view.items || [];
  const pending = view.pending || [];
  const buckets = [
    { k: t("bucketAvailable"), v: tabularMinor(view.buckets?.available_minor), hint: t("bucketAvailableHint") },
    { k: t("bucketHold"), v: tabularMinor(view.buckets?.reserved_minor), hint: t("bucketHoldHint") },
    { k: t("bucketWithdraw"), v: tabularMinor(view.buckets?.withdrawable_minor), hint: t("bucketWithdrawHint") },
  ];
  const totals = [
    { k: t("totRequests"), v: tabularMinor(view.usage_totals?.requests) },
    { k: t("totUsage"), v: tabularMinor(view.usage_totals?.customer_minor) },
    { k: t("totCharge"), v: tabularMinor(view.usage_totals?.charge_minor) },
    { k: t("totPending"), v: tabularMinor(view.usage_totals?.pending_count) },
  ];

  return (
    <div className="flex flex-col gap-6">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label={t("bucketsLabel")}>
        <p className="mb-4 text-sm text-ink-secondary">{t("lead")}</p>
        <div className="grid gap-4 sm:grid-cols-3">
          {buckets.map((card) => (
            <div key={card.k} className="rounded-card border border-hairline bg-canvas p-4">
              <p className="th-eyebrow text-ink-mute">{card.k}</p>
              <p className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
              <p className="mt-2 text-sm text-ink-secondary">{card.hint}</p>
            </div>
          ))}
        </div>
        <section className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-4" aria-label={t("totalsLabel")}>
          {totals.map((card) => (
            <div key={card.k} className="rounded-card border border-hairline bg-canvas p-4">
              <p className="th-eyebrow text-ink-mute">{card.k}</p>
              <p className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
            </div>
          ))}
        </section>
        <div className="mt-4">
          <Button type="button" variant="outline" size="sm" onClick={() => void refresh()}>
            {t("refresh")}
          </Button>
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>

      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="mb-3 text-lg font-semibold tracking-tight">{t("diffTitle")}</h2>
        <ScrollTable
          density="ledger"
          className="rounded-card border border-hairline"
          minWidthClassName="min-w-[56rem]"
          getRowId={(row) => diffKey(row) || "row"}
          rows={items}
          rowClassName={rowClassName}
          empty={
            loaded ? (
              <EmptyLedger title={emptyUsageTitle()} detail={t("emptyDetail")} />
            ) : (
              <p className="text-sm text-ink-mute">{t("loading")}</p>
            )
          }
          columns={[
            {
              id: "request",
              header: "Request",
              cell: (row) => <span className="font-mono text-xs">{row.request_id || "—"}</span>,
            },
            {
              id: "time",
              header: t("colTime"),
              cell: (row) => <span className="font-mono text-xs">{row.occurred_at || "—"}</span>,
            },
            {
              id: "model",
              header: t("colModel"),
              cell: (row) => <span className="font-mono text-xs">{row.public_model_id || "—"}</span>,
            },
            {
              id: "usage",
              header: t("colUsage"),
              cell: (row) => <span className="font-mono tabular-nums">{tabularMinor(row.usage_minor)}</span>,
            },
            {
              id: "charge",
              header: t("colCharge"),
              cell: (row) => <span className="font-mono tabular-nums">{tabularMinor(row.charge_minor)}</span>,
            },
            {
              id: "hold",
              header: t("colHold"),
              cell: (row) => <span className="font-mono tabular-nums">{tabularMinor(row.reserved_minor)}</span>,
            },
            {
              id: "status",
              header: t("colStatus"),
              cell: (row) => (
                <span className="th-eyebrow" data-testid={isMatchRow(row) ? "diff-match" : "diff-mismatch"}>
                  {isMatchRow(row) ? t("matchOk") : t("mismatch")}
                </span>
              ),
            },
            {
              id: "upstream",
              header: t("colUpstreamFacts"),
              cell: (row) => <UpstreamFactsBadge facts={fromDiffRow(row)} />,
            },
            {
              id: "action",
              header: t("colAction"),
              cell: (row) =>
                reconcileActions(row).includes("flag_pending") ? (
                  <SealConfirm
                    size="sm"
                    title={t("flagTitle")}
                    description={t("flagDesc")}
                    onConfirm={() => flag(row)}
                  >
                    {t("flag")}
                  </SealConfirm>
                ) : (
                  <span className="text-sm text-success">{t("matchOk")}</span>
                ),
            },
          ]}
        />
      </section>

      {pending.length > 0 ? (
        <section className="rounded-card border border-hairline bg-canvas-raised p-6">
          <h2 className="mb-3 text-lg font-semibold tracking-tight">{pendingQueueTitle()}</h2>
          <p className="mb-3 text-sm text-ink-secondary">{t("pendingLead")}</p>
          <ul className="space-y-2 font-mono text-xs">
            {pending.map((row) => (
              <li key={diffKey(row)}>{row.request_id || diffKey(row)}</li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
