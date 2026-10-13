"use client";
import { useTranslations } from "next-intl";
import { AdminListPanel } from "@/app/admin/list-panel";
import { formatUsdMinor } from "@/lib/money";
export default function ChannelCommissions() {
  const tUI = useTranslations();
  return <AdminListPanel path="/channel/commissions" title={tUI("consoleCopy.referralCommission")} columns={[
    { accessorKey: 'kind', header: tUI("blogUi.source"), cell: ({ row }) => row.original.kind === 'direct' ? tUI("consoleCopy.directReferral") : row.original.kind === 'indirect' ? tUI("consoleCopy.indirectReferral") : String(row.original.kind || '—') },
    { accessorKey: 'amount_minor', header: tUI("referral.amount"), cell: ({ row }) => formatUsdMinor(Number(row.original.amount_minor)) },
    { accessorKey: 'status', header: tUI("compareUi.status"), cell: ({ row }) => ({ frozen: tUI("referral.frozen"), available: tUI("channelHero.settleable"), held: tUI("referralAccount.income_held"), settled: tUI("referral.settled"), paid: tUI("partnerBoard.settlement_paid"), reversed: tUI("referral.reversed") }[String(row.original.status)] || String(row.original.status || '—')) },
    { accessorKey: 'request_id', header: tUI("consoleCopy.originalUsageRequest") },
  ]} />;
}
