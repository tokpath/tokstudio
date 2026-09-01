"use client";

import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";

type Payment = {
  id: string;
  user_id: string;
  channel_org_id?: string;
  adapter: string;
  purpose: string;
  status: string;
  amount_minor: number;
};

export default function AdminPaymentsPage() {
  const [orderID, setOrderID] = useState("");
  const [message, setMessage] = useState("手工确认和退款都要带二次确认头。");

  async function act(action: "confirm" | "refund") {
    if (!orderID) {
      setMessage("先填写支付单 ID");
      return;
    }
    const res = await fetch(`${apiBase}/admin/payments/${orderID}/${action}`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: "{}",
    });
    const body = await res.json();
    setMessage(res.ok ? `已${action === "confirm" ? "确认" : "退款"} ${body.item?.id} → ${body.item?.status}` : body.error?.message || "操作失败");
  }

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="payments" className="mb-3 text-xl font-medium" />
        <p className="mb-3 text-sm text-ink-secondary">跨渠道订单检索与退款审计。商户配置在各渠道台 /channel/payments，这里不是全站支付设置。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-72" value={orderID} onChange={(e) => setOrderID(e.target.value)} aria-label="支付单 ID" placeholder="pay_..." />
          <ConfirmButton size="sm" title="确认支付入账" description="手工确认后会给用户入账对应权益。" onConfirm={() => act("confirm")}>
            确认支付
          </ConfirmButton>
          <ConfirmButton size="sm" variant="outline" title="确认退款" description="退款会冲正未用完的权益。" onConfirm={() => act("refund")}>
            退款
          </ConfirmButton>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <AdminListPanel<Payment>
        path="/admin/payments"
        title="支付单"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "channel_org_id", header: "渠道" },
          { accessorKey: "adapter", header: "Adapter" },
          { accessorKey: "purpose", header: "Purpose" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "amount_minor", header: "Amount" },
        ]}
      />
    </AdminShell>
  );
}
