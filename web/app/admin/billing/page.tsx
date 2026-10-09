"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { formatUsdMinor } from "@/lib/money";
import { ChargeRefundPanel } from "./charge-refund-panel";
import { BonusPanel } from "./bonus-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { useViewer } from "@/components/rbac/viewer-context";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import { AdminSupplierPanel } from "./supplier-panel";

export default function AdminBillingPage() {
  const t=useTranslations("billingOverview");
  const viewer=useViewer();
  const query = useQuery({
    queryKey: [viewer.userId,"billing-report"],
    queryFn: async () => {
      const response=await fetch(`${apiBase}/admin/billing/report`,{credentials:"include"});
      const body=await response.json() as {report?:Record<string,number>;error?:{message?:string}};
      if(!response.ok || body.error || !body.report)throw new Error(body.error?.message || t("failed"));
      return body;
    },retry:false,
  });
  const report = query.data?.report;

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 level={1} k="billing" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">{t("taskHint")}</p>
        <IfCan action="billing.refund">
          <ChargeRefundPanel onRefund={() => { void query.refetch(); }} />
          <p className="my-3 text-sm"><Link href="/admin/payments" className="text-brand-emphasis underline">{t("payments")}</Link></p>
        </IfCan>
        <IfCan action="billing.bonus"><BonusPanel /></IfCan>

        <h3 className="mt-6 text-base font-semibold">{t("summary")}</h3>
        <p className="mt-2 text-sm text-ink-secondary">{t("scope")}</p>
        {query.isLoading ? <p role="status">{t("loading")}</p> : query.isError || !report ? (
          <div role="alert" className="mt-3"><p className="text-danger">{query.error?.message || t("failed")}</p><Button variant="outline" className="mt-2" onClick={()=>void query.refetch()}>{t("retry")}</Button></div>
        ) : <dl className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {([
            ["revenue_minor", "terminalConsumption"], ["upstream_cost_minor", "serviceCost"],
            ["wholesale_minor", "brandSettlement"], ["commission_liability_minor", "commissionPending"], ["commission_expense_minor", "commissionCost"],
            ["refund_minor", "reversed"], ["gross_profit_minor", "technicalDifference"],
          ] as const).map(([key, label]) => <div key={key} className="rounded-control border border-hairline p-3">
            <dt className="text-sm text-ink-secondary">{t(label)}</dt>
            <dd className="mt-1 font-mono tabular-nums">{formatUsdMinor(report[key])} USD</dd>
          </div>)}
          <div className="rounded-control border border-hairline p-3"><dt className="text-sm text-ink-secondary">{t("pending")}</dt><dd className="mt-1 font-mono">{report.pending_reconciliation_count ?? "—"}</dd></div>
        </dl>}
      </section>
      <AdminSupplierPanel />
    </AdminShell>
  );
}
