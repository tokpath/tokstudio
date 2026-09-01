"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { KeyRound, Lock, Receipt, Wallet } from "lucide-react";
import { Button } from "@/components/ui/button";
import { MetricCard } from "@/components/feature-card";
import { apiBase } from "@/lib/api";
import { useTranslations } from "next-intl";

type Balance = {
  available?: string;
  reserved?: string;
};

function money(value?: string) {
  if (value == null || value === "") return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return value;
  return `$${n.toFixed(2)}`;
}

export function OverviewHero() {
  const t = useTranslations("overview");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [keyCount, setKeyCount] = useState<number | null>(null);
  const [lastReceipt, setLastReceipt] = useState("—");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      const [balRes, keyRes, usageRes] = await Promise.all([
        fetch(`${apiBase}/v1/me/balance`, { credentials: "include" }),
        fetch(`${apiBase}/v1/me/api-keys`, { credentials: "include" }),
        fetch(`${apiBase}/v1/me/usage`, { credentials: "include" }),
      ]);
      if (cancelled) return;
      if (balRes.ok) {
        const body = await balRes.json();
        setBalance(body.balance || null);
      }
      if (keyRes.ok) {
        const body = await keyRes.json();
        setKeyCount(Array.isArray(body.items) ? body.items.length : 0);
      }
      if (usageRes.ok) {
        const body = await usageRes.json();
        const items = Array.isArray(body.items) ? body.items : [];
        const last = items[0] as { public_model_id?: string; state?: string; request_id?: string } | undefined;
        if (last) {
          setLastReceipt([last.public_model_id, last.state, last.request_id].filter(Boolean).join(" · ") || t("hasReceipt"));
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  const cards = [
    { t: t("available"), d: t("availableHint"), v: money(balance?.available), href: "/app/wallet", icon: Wallet, compact: false },
    { t: t("reserved"), d: t("reservedHint"), v: money(balance?.reserved), href: "/app/wallet", icon: Lock, compact: false },
    { t: t("keys"), d: t("keysHint"), v: keyCount == null ? "—" : String(keyCount), href: "/app/keys", icon: KeyRound, compact: false },
    { t: t("receipt"), d: t("receiptHint"), v: lastReceipt, href: "/app/activity", icon: Receipt, compact: true },
  ];

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-2">
        <Button asChild>
          <Link href="/app/keys">
            <KeyRound />
            {t("createKey")}
          </Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/app/wallet">
            <Wallet />
            {t("topup")}
          </Link>
        </Button>
      </div>
      <section className="grid gap-3 md:grid-cols-2 lg:grid-cols-4" aria-label={t("region")}>
        {cards.map((card) => (
          <MetricCard
            key={card.t}
            href={card.href}
            icon={card.icon}
            label={card.t}
            value={card.v}
            hint={card.d}
            compact={card.compact}
          />
        ))}
      </section>
      <p className="text-sm text-ink-mute">{t("footnote")}</p>
    </div>
  );
}
