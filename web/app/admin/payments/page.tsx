"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type Payment = {
  id: string;
  user_id: string;
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
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: "{}",
    });
    const body = await res.json();
    setMessage(res.ok ? `已${action === "confirm" ? "确认" : "退款"} ${body.item?.id} → ${body.item?.status}` : body.error?.message || "操作失败");
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">支付</h2>
        <p className="mb-3 text-sm text-slate-400">查看沙箱支付单，手工入账或退款。退款会冲正未用完的权益。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-72" value={orderID} onChange={(e) => setOrderID(e.target.value)} aria-label="支付单 ID" placeholder="pay_..." />
          <Button size="sm" onClick={() => act("confirm")}>
            确认支付
          </Button>
          <Button size="sm" variant="outline" onClick={() => act("refund")}>
            退款
          </Button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
      <AdminListPanel<Payment>
        path="/admin/payments"
        title="支付单"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "adapter", header: "Adapter" },
          { accessorKey: "purpose", header: "Purpose" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "amount_minor", header: "Amount" },
        ]}
      />
    </AdminShell>
  );
}
