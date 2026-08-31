"use client";

import { useEffect, useState } from "react";
import { apiBase } from "@/lib/api";

function micro(n: unknown) {
  const v = Number(n);
  if (!Number.isFinite(v)) return "—";
  return `$${(v / 1_000_000).toFixed(2)}`;
}

export function ChannelHero() {
  const [quota, setQuota] = useState("—");
  const [consumed, setConsumed] = useState("—");
  const [frozen, setFrozen] = useState("—");
  const [settleable, setSettleable] = useState("—");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      const [q, c, s] = await Promise.all([
        fetch(`${apiBase}/channel/quota`, { credentials: "include" }),
        fetch(`${apiBase}/channel/commissions`, { credentials: "include" }),
        fetch(`${apiBase}/channel/settlements`, { credentials: "include" }),
      ]);
      if (cancelled) return;
      if (q.ok) {
        const body = await q.json();
        setQuota(micro(body.quota?.available_minor));
        setConsumed(micro(body.quota?.consumed_minor));
      }
      if (c.ok) {
        const body = await c.json();
        const items = Array.isArray(body.items) ? body.items : [];
        const hold = items.filter((i: { status?: string }) => i.status === "frozen" || i.status === "hold");
        const sum = hold.reduce((n: number, i: { amount_minor?: number }) => n + Number(i.amount_minor || 0), 0);
        setFrozen(micro(sum));
      }
      if (s.ok) {
        const body = await s.json();
        const items = Array.isArray(body.items) ? body.items : [];
        const open = items.filter((i: { status?: string }) => i.status !== "paid");
        const sum = open.reduce((n: number, i: { amount_minor?: number }) => n + Number(i.amount_minor || 0), 0);
        setSettleable(micro(sum));
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  const cards = [
    { t: "可用额度", d: "本渠道还能发放的服务额度", v: quota },
    { t: "已消费", d: "下属用户已经打出去的账", v: consumed },
    { t: "佣金冻结", d: "HOLD 中尚未到期", v: frozen },
    { t: "可结算", d: "结算单未打款金额", v: settleable },
  ];

  return (
    <section className="grid gap-3 md:grid-cols-2 lg:grid-cols-4" aria-label="渠道总览">
      {cards.map((card) => (
        <div key={card.t} className="rounded-card border border-hairline bg-canvas-raised p-4">
          <p className="th-eyebrow text-ink-mute">{card.t}</p>
          <p className="mt-2 font-mono text-[28px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
          <p className="mt-2 text-sm text-ink-secondary">{card.d}</p>
        </div>
      ))}
    </section>
  );
}
