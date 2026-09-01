"use client";

import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { ChannelPaymentsNav } from "../payments-nav";
import { AdminListPanel } from "@/app/admin/list-panel";

type Order = {
  id: string;
  user_id: string;
  adapter: string;
  purpose: string;
  status: string;
  amount_minor: number;
  credit_minor: number;
  currency: string;
};

export default function ChannelPaymentOrdersPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelPaymentOrders" />
      <ChannelPaymentsNav />
      <AdminListPanel<Order>
        path="/channel/payments/orders"
        title="本渠道支付单"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "user_id", header: "用户" },
          { accessorKey: "adapter", header: "通道" },
          { accessorKey: "currency", header: "币种" },
          { accessorKey: "amount_minor", header: "应付" },
          { accessorKey: "credit_minor", header: "到账" },
          { accessorKey: "status", header: "状态" },
        ]}
      />
    </div>
  );
}
