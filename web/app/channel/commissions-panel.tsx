"use client";
import { AdminListPanel } from "@/app/admin/list-panel";
import { formatUsdMinor } from "@/lib/money";
export default function ChannelCommissions() {
  return <AdminListPanel path="/channel/commissions" title="推广佣金" columns={[
    { accessorKey: 'kind', header: '来源', cell: ({ row }) => row.original.kind === 'direct' ? '直接邀请' : row.original.kind === 'indirect' ? '间接邀请' : String(row.original.kind || '—') },
    { accessorKey: 'amount_minor', header: '金额（USD）', cell: ({ row }) => formatUsdMinor(Number(row.original.amount_minor)) },
    { accessorKey: 'status', header: '状态', cell: ({ row }) => ({ frozen: '冻结中', available: '可结算', held: '暂停结算', settled: '已生成结算单', paid: '已登记打款', reversed: '已冲正' }[String(row.original.status)] || String(row.original.status || '—')) },
    { accessorKey: 'request_id', header: '原消费请求' },
  ]} />;
}
