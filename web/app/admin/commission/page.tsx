"use client";

import { useState } from "react";
import { AdminShell } from "../shell";
import { AdminListPanel } from "../list-panel";
import { SettlementPanel } from "./settlement-panel";
import { RecoveryPanel } from "./recovery-panel";
import { RecalcPanel } from "./recalc-panel";
import { PolicyEditor } from "./policy-editor";
import { IfCan } from "@/components/rbac/if-can";
import { useViewer } from "@/components/rbac/viewer-context";
import { canWrite } from "@/lib/rbac";
import { formatUsdMinor } from "@/lib/money";
import { Button } from "@/components/ui/button";

type Commission = { id: string; kind: string; status: string; amount_minor: number; channel_org_id?: string };
export default function AdminCommissionPage() {
  const viewer = useViewer();
  const [tab, setTab] = useState("settlements");
  const [version, setVersion] = useState(0);
  return <AdminShell>
    <nav aria-label="佣金工作区" className="flex flex-wrap gap-2">
      {[['settlements', '结算与打款'], ['recovery', '待收回'], ['records', '全部佣金'], ['rules', '规则'], ['verify', '消费核对']].filter(([key]) => (key !== 'recovery' || canWrite('commission.recovery.read', viewer)) && (key !== 'verify' || canWrite('commission.write', viewer))).map(([key, label]) => <Button key={key} variant={tab === key ? 'default' : 'outline'} aria-pressed={tab === key} onClick={() => setTab(key)}>{label}</Button>)}
    </nav>
    {tab === 'settlements' ? <SettlementPanel key={version} /> : null}
    {tab === 'recovery' ? <IfCan action="commission.recovery.read"><RecoveryPanel onRecorded={() => setVersion(v => v + 1)} /></IfCan> : null}
    {tab === 'rules' ? <PolicyEditor canEdit={canWrite('commission.write', viewer)} /> : null}
    {tab === 'verify' ? <IfCan action="commission.write"><RecalcPanel /></IfCan> : null}
    {tab === 'records' ? <AdminListPanel<Commission> path="/admin/commissions" title="佣金明细" columns={[
      { accessorKey: 'kind', header: '佣金类型', cell: ({ row }) => ({ direct: '直接佣金', indirect: '间接佣金' }[row.original.kind] || row.original.kind) },
      { accessorKey: 'status', header: '状态', cell: ({ row }) => ({ frozen: '冻结中', available: '可结算', held: '暂停结算', settled: '已生成结算单', paid: '已登记打款', reversed: '已冲正' }[row.original.status] || row.original.status) },
      { accessorKey: 'amount_minor', header: '金额（USD）', cell: ({ row }) => formatUsdMinor(row.original.amount_minor) },
      { accessorKey: 'channel_org_id', header: '推广归属' },
    ]} /> : null}
  </AdminShell>;
}
