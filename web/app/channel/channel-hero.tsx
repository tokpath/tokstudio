"use client";

import { useEffect, useState } from "react";
import { apiBase } from "@/lib/api";
import { useTranslations } from "next-intl";
import { MetricCard } from "@/components/feature-card";
import { CHANNEL_HERO_ICONS } from "@/lib/page-icons";

function micro(n: unknown) {
  const v = Number(n);
  if (!Number.isFinite(v)) return "—";
  return `$${(v / 1_000_000).toFixed(2)}`;
}

export function ChannelHero() {
  const t = useTranslations("channelHero");
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
    { t: t("quota"), d: t("quotaHint"), v: quota, icon: CHANNEL_HERO_ICONS[0] },
    { t: t("consumed"), d: t("consumedHint"), v: consumed, icon: CHANNEL_HERO_ICONS[1] },
    { t: t("frozen"), d: t("frozenHint"), v: frozen, icon: CHANNEL_HERO_ICONS[2] },
    { t: t("settleable"), d: t("settleableHint"), v: settleable, icon: CHANNEL_HERO_ICONS[3] },
  ];

  return (
    <section className="grid gap-3 md:grid-cols-2 lg:grid-cols-4" aria-label={t("region")}>
        {cards.map((card) => (
          <MetricCard key={card.t} icon={card.icon} label={card.t} value={card.v} hint={card.d} />
        ))}
    </section>
  );
}
