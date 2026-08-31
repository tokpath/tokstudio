"use client";

import { useState } from "react";
import { apiBase } from "@/lib/api";

type Balance = {
  available?: string;
  reserved?: string;
  available_minor?: number;
  reserved_minor?: number;
};

export default function WalletPanel() {
  const [balance, setBalance] = useState<Balance | null>(null);
  const [code, setCode] = useState("THE2E");
  const [message, setMessage] = useState("登录后可查看余额；M3 用兑换码 THE2E 充值。");

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/balance`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录");
      return;
    }
    setBalance(body.balance);
    setMessage("余额已刷新");
  }

  async function redeem() {
    const response = await fetch(`${apiBase}/v1/topups/redeem`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });
    const body = await response.json();
    setMessage(response.ok ? `兑换成功 ${body.item?.amount_minor} micro-USD` : body.error?.message || "兑换失败");
    if (response.ok) {
      await refresh();
    }
  }

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6 ">
      <h2 className="mb-3 text-xl font-medium tracking-tight">现金钱包</h2>
      <p className="mb-4 text-sm text-ink-secondary">
        可用 {balance?.available ?? "—"} USD，预授权占用 {balance?.reserved ?? "0"} USD。
      </p>
      <div className="flex flex-wrap gap-3">
        <button className="rounded border border-hairline px-4 py-2" onClick={refresh}>
          刷新余额
        </button>
        <input className="rounded bg-canvas px-3 py-2" value={code} onChange={(e) => setCode(e.target.value)} />
        <button className="rounded px-4 py-2 text-on-brand" style={{ background: "var(--brand-primary)" }} onClick={redeem}>
          兑换
        </button>
      </div>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
