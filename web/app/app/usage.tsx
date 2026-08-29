"use client";

import { useState } from "react";
import { apiBase } from "@/lib/api";

type UsageRow = {
  id: string;
  request_id?: string;
  state?: string;
  customer_amount_minor?: number;
  public_model_id?: string;
};

type LedgerRow = {
  id: string;
  entry_type?: string;
  amount_minor?: number;
};

export default function UsagePanel() {
  const [usage, setUsage] = useState<UsageRow[]>([]);
  const [ledger, setLedger] = useState<LedgerRow[]>([]);
  const [message, setMessage] = useState("登录后可查看自己的用量和账单流水。");

  async function refresh() {
    const [usageRes, ledgerRes] = await Promise.all([
      fetch(`${apiBase}/v1/me/usage`, { credentials: "include" }),
      fetch(`${apiBase}/v1/me/ledger`, { credentials: "include" }),
    ]);
    const usageBody = await usageRes.json();
    const ledgerBody = await ledgerRes.json();
    if (!usageRes.ok) {
      setMessage(usageBody.error?.message || "未登录");
      return;
    }
    setUsage(usageBody.items || []);
    setLedger(ledgerBody.items || []);
    setMessage("用量与账单已刷新");
  }

  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <h2 className="mb-3 text-xl font-medium">用量与账单</h2>
      <p className="mb-4 text-sm text-slate-400">只展示当前登录用户的 usage 和账本，不含其他渠道数据。</p>
      <button className="mb-4 rounded border border-slate-600 px-4 py-2" onClick={refresh}>
        刷新账单
      </button>
      <p className="text-sm text-slate-300">
        usage {usage.length} 条，流水 {ledger.length} 条。{message}
      </p>
    </section>
  );
}
