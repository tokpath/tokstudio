"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";

type PnL = {
  recharge_minor?: number;
  unconsumed_minor?: number;
  consumed_minor?: number;
  marketing_minor?: number;
  supplier_minor?: number;
  pnl_minor?: number;
};

function micro(n: unknown) {
  const v = Number(n);
  if (!Number.isFinite(v)) return "—";
  return `$${(v / 1_000_000).toFixed(2)}`;
}

export function ChannelPnLPanel({ channelID }: { channelID: string }) {
  const [pnl, setPnl] = useState<PnL>({});
  const [message, setMessage] = useState("盈亏 = 已消费 + 营销 + 供应商支出。未消费不计入利润。");

  async function load() {
    const res = await fetch(`${apiBase}/admin/channels/${encodeURIComponent(channelID)}/pnl`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "读取盈亏失败");
      return;
    }
    setPnl((body.pnl || {}) as PnL);
    setMessage(`已读取 ${channelID}`);
  }

  const rows = [
    ["充值（含未消费）", pnl.recharge_minor],
    ["未消费", pnl.unconsumed_minor],
    ["已消费", pnl.consumed_minor],
    ["营销", pnl.marketing_minor],
    ["供应商支出", pnl.supplier_minor],
    ["盈亏", pnl.pnl_minor],
  ] as const;

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
      <h2 className="mb-4 text-lg font-semibold tracking-tight">渠道盈亏</h2>
      <p className="mb-3 text-sm text-ink-secondary">各积分池单独算。C 划给下属 B 的额度记在 B 的池里。</p>
      <dl className="mb-3 grid gap-2 sm:grid-cols-2">
        {rows.map(([label, value]) => (
          <div key={label} className="flex justify-between gap-4 border-b border-hairline py-2">
            <dt className="text-ink-secondary">{label}</dt>
            <dd className="font-mono tabular-nums">{micro(value)}</dd>
          </div>
        ))}
      </dl>
      <Button size="sm" variant="outline" onClick={() => void load()}>
        读取盈亏
      </Button>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
