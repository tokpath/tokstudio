"use client";

import Link from "next/link";
import { useMemo } from "react";
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
import { ActionRow, LeadActions } from "@/components/console/action-row";
import { ListResourceView } from "@/components/console/list-resource-view";
import { MetricCard } from "@/components/feature-card";
import { UsageCharts } from "@/components/usage-charts";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems, type ListLoadResult, type ListSnapshot } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";
import { overviewHasUsage, overviewNeedsTopup } from "@/lib/overview-guide";
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

async function loadBalance(): Promise<ListLoadResult<Balance>> {
  try {
    const response = await fetch(`${apiBase}/v1/me/balance`, { credentials: "include" });
    const body: unknown = await response.json().catch(() => ({}));
    const record = body && typeof body === "object" ? (body as { balance?: Balance; error?: { message?: string; code?: string } }) : {};
    if (!response.ok) {
      return { ok: false, status: response.status, items: [], message: record.error?.message, code: record.error?.code };
    }
    return { ok: true, status: response.status, items: record.balance ? [record.balance] : [] };
  } catch {
    return { ok: false, network: true, items: [] };
  }
}

function metricDisplay(snapshot: ListSnapshot<unknown>, ready: string) {
  if (snapshot.phase === "ready" || snapshot.phase === "stale" || snapshot.phase === "empty") {
    return ready;
  }
  return "—";
}

/** 总览英雄：各数据区独立加载；只有用量成功且零条才给第一次使用步骤。 */
export function OverviewHero() {
  const t = useTranslations("overview");
  const tc = useTranslations("common");
  const tChart = useTranslations("charts");
  const balanceList = useListResource<Balance>({ load: loadBalance });
  const keysList = useListResource<{ id?: string }>({
    load: () => fetchListItems(`${apiBase}/v1/me/api-keys`),
  });
  const usageList = useListResource<UsageEvent>({
    load: () => fetchListItems(`${apiBase}/v1/me/usage?limit=100`),
  });

  const balance = balanceList.snapshot.items[0];
  const events = usageList.snapshot.items;
  const summary = useMemo(() => summarizeUsage(events), [events]);
  const tokens = summary.prompt + summary.completion + summary.reasoning;
  const usageOk = usageList.snapshot.phase === "empty" || usageList.snapshot.phase === "ready" || usageList.snapshot.phase === "stale";
  const hasUsage = usageOk && overviewHasUsage(events);
  const needsTopup = overviewNeedsTopup(balance?.available);
  const usageEmpty = usageList.snapshot.phase === "empty";

  const cards = [
    {
      t: t("available"),
      d: t("availableHint"),
      v: metricDisplay(balanceList.snapshot, money(balance?.available)),
      href: "/app/wallet",
      icon: Wallet,
      compact: false,
      snapshot: balanceList.snapshot,
      retry: () => void balanceList.reload(),
      name: "balance-available",
    },
    {
      t: t("reserved"),
      d: t("reservedHint"),
      v: metricDisplay(balanceList.snapshot, money(balance?.reserved)),
      href: "/app/wallet",
      icon: Lock,
      compact: false,
      snapshot: balanceList.snapshot,
      retry: () => void balanceList.reload(),
      name: "balance-reserved",
    },
    {
      t: t("keys"),
      d: t("keysHint"),
      v: metricDisplay(keysList.snapshot, keysList.snapshot.phase === "empty" ? "0" : String(keysList.snapshot.items.length)),
      href: "/app/keys",
      icon: KeyRound,
      compact: false,
      snapshot: keysList.snapshot,
      retry: () => void keysList.reload(),
      name: "keys",
    },
    {
      t: t("receipt"),
      d: t("receiptHint"),
      v: metricDisplay(
        usageList.snapshot,
        events[0]
          ? [events[0].public_model_id, events[0].state, events[0].request_id].filter(Boolean).join(" · ") || t("hasReceipt")
          : "—",
      ),
      href: "/app/activity",
      icon: Receipt,
      compact: true,
      snapshot: usageList.snapshot,
      retry: () => void usageList.reload(),
      name: "receipt",
    },
  ];

  const periodReady = usageOk;
  const periodCards = [
    { k: t("periodRequests"), v: periodReady ? String(summary.requests) : "—" },
    { k: t("periodTokens"), v: periodReady ? String(tokens) : "—" },
    { k: t("periodSpend"), v: periodReady && events.length > 0 ? formatUsdMinor(summary.amount) : "—" },
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
      {usageOk ? (
        <ActionRow className="gap-3">
          {hasUsage ? (
            <>
              <Button asChild>
                <Link href="/app/keys?create=1">
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
            </>
          ) : (
            <>
              <Button asChild>
                <Link href="/app/playground">
                  <Play />
                  {t("tryCta")}
                </Link>
              </Button>
              <Button asChild variant="outline">
                <Link href="/app/keys?create=1">
                  <KeyRound />
                  {t("createKey")}
                </Link>
              </Button>
              {needsTopup ? (
                <Button asChild variant="outline">
                  <Link href="/app/wallet">
                    <Wallet />
                    {t("topup")}
                  </Link>
                </Button>
              ) : null}
            </>
          )}
        </ActionRow>
      ) : null}

      <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-4" aria-label={t("region")}>
        {cards.map((card) => {
          const failed = card.snapshot.phase === "error" || card.snapshot.phase === "unauthorized";
          return (
            <div key={card.t} data-testid={`overview-metric-${card.name}`} data-list-phase={card.snapshot.phase}>
              <MetricCard
                href={card.href}
                icon={card.icon}
                label={card.t}
                value={card.v}
                hint={failed ? card.snapshot.message || tc("listFailed") : card.d}
                compact={card.compact}
              />
              {failed ? (
                <Button type="button" size="sm" variant="outline" className="mt-2" onClick={card.retry}>
                  {tc("listRetry")}
                </Button>
              ) : null}
            </div>
          );
        })}
      </section>

      <section aria-label={t("trendsRegion")} className="flex flex-col gap-3">
        <LeadActions
          className="mb-0"
          lead={
            <>
              <h2 className="text-sm font-semibold tracking-tight text-ink">{t("trendsTitle")}</h2>
              <p className="mt-1 text-[13px] leading-relaxed text-ink-mute">{t("trendsLead")}</p>
              <p className="mt-1 text-[13px] leading-relaxed text-ink-mute">{t("trendsScope")}</p>
            </>
          }
          actions={
            <Button asChild variant="outline" size="sm">
              <Link href="/app/usage">{t("trendsToUsage")}</Link>
            </Button>
          }
        />
        <ListResourceView
          snapshot={usageList.snapshot}
          emptyTitle={tChart("empty")}
          emptyDetail={tChart("emptyDetail")}
          loadingTitle={t("trendsTitle")}
          onRetry={() => void usageList.reload()}
          name="overview-usage"
          passEmpty
        >
          <div className="grid gap-4 sm:grid-cols-3">
            {periodCards.map((card) => (
              <div key={card.k} className="rounded-card border border-hairline bg-canvas p-4">
                <p className="th-eyebrow text-ink-mute">{card.k}</p>
                <p className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
              </div>
            ))}
          </div>
          <UsageCharts events={events} breakdownTitle={tChart("byModel")} testIdPrefix="overview" />
        </ListResourceView>
      </section>

      {usageEmpty ? (
        <section aria-label={t("firstUseRegion")} className="flex flex-col gap-3">
          <h2 className="text-sm font-semibold tracking-tight text-ink">{t("firstUseTitle")}</h2>
          <p className="text-[13px] leading-relaxed text-ink-mute">{t("firstUseLead")}</p>
          <div className="grid gap-3 lg:grid-cols-2">
            <article className="flex flex-col gap-3 rounded-card border border-hairline bg-canvas-raised p-4">
              <h3 className="text-sm font-semibold tracking-tight text-ink">{t("tryTitle")}</h3>
              <p className="text-[13px] leading-relaxed text-ink-mute">{t("trySteps")}</p>
              {needsTopup ? <p className="text-[13px] leading-relaxed text-danger">{t("tryNeedTopup")}</p> : null}
              <ActionRow className="gap-2">
                <Button asChild>
                  <Link href="/app/playground">{t("tryCta")}</Link>
                </Button>
                {needsTopup ? (
                  <Button asChild variant="outline">
                    <Link href="/app/wallet">{t("topup")}</Link>
                  </Button>
                ) : null}
              </ActionRow>
            </article>
            <article className="flex flex-col gap-3 rounded-card border border-hairline bg-canvas-raised p-4">
              <h3 className="text-sm font-semibold tracking-tight text-ink">{t("integrateTitle")}</h3>
              <p className="text-[13px] leading-relaxed text-ink-mute">{t("integrateSteps")}</p>
              <ActionRow className="gap-2">
                <Button asChild>
                  <Link href="/app/keys?create=1">{t("createKey")}</Link>
                </Button>
                <Button asChild variant="outline">
                  <Link href="/app/docs">{t("copyConfigCta")}</Link>
                </Button>
              </ActionRow>
            </article>
          </div>
        </section>
      ) : null}

      {hasUsage ? (
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
      ) : null}

      <p className="text-[13px] leading-relaxed text-ink-mute">{t("footnote")}</p>
    </div>
  );
}
