"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import {
  BarChart3,
  BookOpen,
  Boxes,
  Clapperboard,
  KeyRound,
  Lock,
  Play,
  Receipt,
  Wallet,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { MetricCard } from "@/components/feature-card";
import { UsageCharts } from "@/components/usage-charts";
import { apiBase } from "@/lib/api";
import { type UsageEvent, summarizeUsage } from "@/lib/usage";
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

/** 总览英雄：个人状态 + 周期用量趋势 + 快捷入口（docs/14，对齐 tokpath dashboard）。 */
export function OverviewHero() {
  const t = useTranslations("overview");
  const tChart = useTranslations("charts");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [keyCount, setKeyCount] = useState<number | null>(null);
  const [lastReceipt, setLastReceipt] = useState("—");
  const [events, setEvents] = useState<UsageEvent[]>([]);
  const [usageReady, setUsageReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      const [balRes, keyRes, usageRes] = await Promise.all([
        fetch(`${apiBase}/v1/me/balance`, { credentials: "include" }),
        fetch(`${apiBase}/v1/me/api-keys`, { credentials: "include" }),
        fetch(`${apiBase}/v1/me/usage?limit=100`, { credentials: "include" }),
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
        const items = (Array.isArray(body.items) ? body.items : []) as UsageEvent[];
        setEvents(items);
        const last = items[0] as { public_model_id?: string; state?: string; request_id?: string } | undefined;
        if (last) {
          setLastReceipt(
            [last.public_model_id, last.state, last.request_id].filter(Boolean).join(" · ") || t("hasReceipt"),
          );
        }
      }
      setUsageReady(true);
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [t]);

  const summary = useMemo(() => summarizeUsage(events), [events]);
  const tokens = summary.prompt + summary.completion + summary.reasoning;

  const cards = [
    { t: t("available"), d: t("availableHint"), v: money(balance?.available), href: "/app/wallet", icon: Wallet, compact: false },
    { t: t("reserved"), d: t("reservedHint"), v: money(balance?.reserved), href: "/app/wallet", icon: Lock, compact: false },
    { t: t("keys"), d: t("keysHint"), v: keyCount == null ? "—" : String(keyCount), href: "/app/keys", icon: KeyRound, compact: false },
    { t: t("receipt"), d: t("receiptHint"), v: lastReceipt, href: "/app/activity", icon: Receipt, compact: true },
  ];

  const periodCards = [
    { k: t("periodRequests"), v: usageReady ? String(summary.requests) : "—" },
    { k: t("periodTokens"), v: usageReady ? String(tokens) : "—" },
    { k: t("periodSpend"), v: usageReady ? String(summary.amount) : "—" },
  ];

  const shortcuts = [
    { href: "/app/wallet", label: t("shortcutWallet"), icon: Wallet },
    { href: "/app/keys", label: t("shortcutKeys"), icon: KeyRound },
    { href: "/app/playground", label: t("shortcutPlayground"), icon: Play },
    { href: "/app/usage", label: t("shortcutUsage"), icon: BarChart3 },
    { href: "/app/activity", label: t("shortcutActivity"), icon: Receipt },
    { href: "/app/media", label: t("shortcutMedia"), icon: Clapperboard },
    { href: "/app/catalog", label: t("shortcutCatalog"), icon: Boxes },
    { href: "/app/docs", label: t("shortcutDocs"), icon: BookOpen },
  ];

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-wrap gap-3">
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

      <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-4" aria-label={t("region")}>
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

      <section aria-label={t("trendsRegion")} className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold tracking-tight text-ink">{t("trendsTitle")}</h2>
            <p className="mt-1 text-[13px] leading-relaxed text-ink-mute">{t("trendsLead")}</p>
          </div>
          <Button asChild variant="outline" size="sm">
            <Link href="/app/usage">{t("trendsToUsage")}</Link>
          </Button>
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          {periodCards.map((card) => (
            <div key={card.k} className="rounded-card border border-hairline bg-canvas p-4">
              <p className="th-eyebrow text-ink-mute">{card.k}</p>
              <p className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
            </div>
          ))}
        </div>
        <UsageCharts events={events} breakdownTitle={tChart("byModel")} testIdPrefix="overview" />
      </section>

      <section aria-label={t("shortcutsRegion")} className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold tracking-tight text-ink">{t("shortcutsTitle")}</h2>
        <p className="text-[13px] leading-relaxed text-ink-mute">{t("shortcutsLead")}</p>
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {shortcuts.map((item) => {
            const Icon = item.icon;
            return (
              <li key={item.href}>
                <Link
                  href={item.href}
                  className="flex items-center gap-2 rounded-control border border-hairline bg-canvas-raised px-3 py-2.5 text-sm text-ink transition-colors hover:border-brand/40 hover:bg-brand-soft/40"
                >
                  <Icon className="size-4 shrink-0 text-ink-mute" strokeWidth={1.75} />
                  <span>{item.label}</span>
                </Link>
              </li>
            );
          })}
        </ul>
      </section>

      <p className="text-[13px] leading-relaxed text-ink-mute">{t("footnote")}</p>
    </div>
  );
}
