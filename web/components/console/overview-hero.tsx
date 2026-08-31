"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
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
          setLastReceipt([last.public_model_id, last.state, last.request_id].filter(Boolean).join(" · ") || "有回单");
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  const cards = [
    { t: t("available"), d: t("availableHint"), v: money(balance?.available), href: "/app/wallet" },
    { t: t("reserved"), d: t("reservedHint"), v: money(balance?.reserved), href: "/app/wallet" },
    { t: t("keys"), d: t("keysHint"), v: keyCount == null ? "—" : String(keyCount), href: "/app/keys" },
    { t: t("receipt"), d: t("receiptHint"), v: lastReceipt, href: "/app/activity" },
  ];

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-2">
        <Button asChild>
          <Link href="/app/keys">{t("createKey")}</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/app/wallet">{t("topup")}</Link>
        </Button>
      </div>
      <section className="grid gap-3 md:grid-cols-2 lg:grid-cols-4" aria-label={t("region")}>
        {cards.map((card) => (
          <Link
            key={card.t}
            href={card.href}
            className="rounded-card border border-hairline bg-canvas-raised p-4 no-underline hover:bg-brand-soft/40"
          >
            <p className="th-eyebrow text-ink-mute">{card.t}</p>
            <p
              className={`mt-2 font-mono font-medium tracking-tight text-ink ${
                card.t === t("receipt") ? "text-sm leading-snug" : "text-[28px] leading-none tabular-nums"
              }`}
            >
              {card.v}
            </p>
            <p className="mt-2 text-sm text-ink-secondary">{card.d}</p>
          </Link>
        ))}
      </section>
      <p className="text-sm text-ink-mute">{t("footnote")}</p>
    </div>
  );
}
