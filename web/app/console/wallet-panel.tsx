"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ActionRow } from "@/components/console/action-row";
import { ListResourceView } from "@/components/console/list-resource-view";
import { CheckoutPay } from "@/components/checkout-pay";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import type { CheckoutPayload } from "@/lib/checkout";
import { formatOrderCredit, formatOrderDue, orderMatchesSelection } from "@/lib/checkout";
import { formatUsdMinor } from "@/lib/money";
import {
  applyQuoteFetch,
  emptyQuoteSnapshot,
  formatAmountChip,
  formatCreditMinor,
  formatPayMinor,
  loadingQuoteSnapshot,
  quoteMatchesSelection,
  type PaymentQuote,
  type QuoteSnapshot,
} from "@/lib/payment-quote";

type Balance = {
  available?: string;
  reserved?: string;
  available_minor?: number;
  reserved_minor?: number;
  gift_minor?: number;
  purchased_minor?: number;
  commission_available_minor?: number;
};

type Method = {
  adapter: string;
  display_name?: string;
  name?: string;
  sandbox?: boolean;
  auto_renew_supported?: boolean;
  brand_color?: string;
  pay_currency?: string;
};

export default function WalletPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [code, setCode] = useState("");
  const [message, setMessage] = useState(t("walletHint"));
  const [help, setHelp] = useState("");
  const [chips, setChips] = useState<number[]>([100, 300, 500, 1000]);
  const [amount, setAmount] = useState(100);
  const [adapter, setAdapter] = useState("");
  const [quoteSnap, setQuoteSnap] = useState<QuoteSnapshot>(emptyQuoteSnapshot);
  const [creating, setCreating] = useState(false);
  const [createUnknown, setCreateUnknown] = useState(false);
  const [checkout, setCheckout] = useState<CheckoutPayload | null>(null);
  const [quoteTick, setQuoteTick] = useState(0);
  const quoteGen = useRef(0);
  const createKeyRef = useRef("");
  const methodsList = useListResource<Method>({
    load: async () => {
      try {
        const response = await fetch(`${apiBase}/v1/payments/checkout`, { credentials: "include" });
        const body = await response.json().catch(() => ({}));
        if (!response.ok) {
          return { ok: false, status: response.status, items: [], message: body.error?.message, code: body.error?.code };
        }
        const item = body.item || {};
        const nextMethods: Method[] = item.methods || [];
        setHelp(item.help_text || "");
        const amounts: number[] = item.settings?.quick_amounts || [100, 300, 500, 1000];
        setChips(amounts);
        if (amounts[0]) setAmount(amounts[0]);
        if (nextMethods[0]) setAdapter(nextMethods[0].adapter);
        return { ok: true, status: response.status, items: nextMethods };
      } catch {
        return { ok: false, network: true, items: [] };
      }
    },
  });
  const methods = methodsList.snapshot.items;

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/balance`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    setBalance(body.balance);
    setMessage(t("walletRefreshed"));
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const generation = ++quoteGen.current;
    if (!adapter || amount <= 0) {
      setQuoteSnap(emptyQuoteSnapshot());
      return;
    }
    setQuoteSnap(loadingQuoteSnapshot());
    const controller = new AbortController();
    void (async () => {
      try {
        const response = await fetch(
          `${apiBase}/v1/payments/quote?adapter=${encodeURIComponent(adapter)}&pay_major=${amount}`,
          { credentials: "include", signal: controller.signal },
        );
        const body = await response.json().catch(() => ({}));
        const next = applyQuoteFetch({
          generation,
          currentGeneration: quoteGen.current,
          adapter,
          amount,
          ok: response.ok,
          quote: body.item as PaymentQuote | undefined,
          message: body.error?.message,
        });
        if (next) {
          setQuoteSnap(next);
        }
      } catch {
        if (controller.signal.aborted) {
          return;
        }
        const next = applyQuoteFetch({
          generation,
          currentGeneration: quoteGen.current,
          adapter,
          amount,
          ok: false,
          message: t("quoteFailed"),
        });
        if (next) {
          setQuoteSnap(next);
        }
      }
    })();
    return () => controller.abort();
  }, [adapter, amount, quoteTick, t]);

  useEffect(() => {
    if (creating || createUnknown) {
      return;
    }
    createKeyRef.current = crypto.randomUUID();
  }, [adapter, amount, creating, createUnknown]);

  async function redeem() {
    const response = await fetch(`${apiBase}/v1/topups/redeem`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });
    const body = await response.json();
    setMessage(response.ok ? t("redeemOk", { amount: formatUsdMinor(body.item?.amount_minor) }) : body.error?.message || t("redeemFail"));
    if (response.ok) await refresh();
  }

  async function pay() {
    if (creating) {
      return;
    }
    if (createUnknown) {
      await submitOrder();
      return;
    }
    const quote = quoteSnap.quote;
    if (!quoteMatchesSelection(quote, adapter, amount)) {
      return;
    }
    await submitOrder();
  }

  async function submitOrder() {
    if (!createKeyRef.current) {
      createKeyRef.current = crypto.randomUUID();
    }
    setCreating(true);
    try {
      const response = await fetch(`${apiBase}/v1/payments/orders`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json", "Idempotency-Key": createKeyRef.current },
        body: JSON.stringify({ adapter, pay_major: amount }),
      });
      const body = await response.json().catch(() => ({}));
      if (response.ok) {
        const next = (body.checkout || null) as CheckoutPayload | null;
        if (next?.order && !next.order.adapter) {
          next.order.adapter = next.adapter;
        }
        setCheckout(next);
        setCreateUnknown(false);
        setMessage(t("payOrderOk", { id: body.checkout?.order?.id }));
        createKeyRef.current = crypto.randomUUID();
      } else {
        setCreateUnknown(false);
        setMessage(body.error?.message || t("payOrderFail"));
      }
    } catch {
      setCreateUnknown(true);
      setMessage(t("payCreateUnconfirmed"));
    } finally {
      setCreating(false);
    }
  }

  const selected = methods.find((m) => m.adapter === adapter);
  const payCurrency = selected?.pay_currency || quoteSnap.quote?.pay_currency;
  const order = checkout?.order;
  const showOrderSummary = orderMatchesSelection(order, adapter, amount);
  const mismatch = Boolean(order && !showOrderSummary && quoteMatchesSelection(quoteSnap.quote, adapter, amount));
  const selectionLocked = creating || createUnknown;
  const canPay =
    createUnknown || (quoteSnap.phase === "ready" && quoteMatchesSelection(quoteSnap.quote, adapter, amount) && !creating);
  const payLabel = creating
    ? t("creatingOrder")
    : createUnknown
      ? t("recoverCreate")
      : quoteSnap.phase === "loading"
        ? t("quoteCalculating")
        : canPay
          ? t("payNow", { money: formatPayMinor(quoteSnap.quote?.pay_currency, quoteSnap.quote?.pay_minor) })
          : t("quoteUnavailable");

  const dueText = showOrderSummary
    ? formatOrderDue(order)
    : quoteSnap.phase === "ready" && quoteSnap.quote
      ? formatPayMinor(quoteSnap.quote.pay_currency, quoteSnap.quote.pay_minor)
      : "—";
  const feeText =
    !showOrderSummary && quoteSnap.phase === "ready" && quoteSnap.quote
      ? formatPayMinor(quoteSnap.quote.pay_currency, quoteSnap.quote.fee_minor)
      : "—";
  const creditText = showOrderSummary
    ? formatOrderCredit(order)
    : quoteSnap.phase === "ready" && quoteSnap.quote
      ? formatCreditMinor(quoteSnap.quote.credit_minor)
      : "—";

  const methodsUsable = methodsList.snapshot.phase === "ready" || methodsList.snapshot.phase === "stale";

  return (
    <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <section className="min-w-0 rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="mb-4 text-sm text-ink-secondary">
          {t("walletMeta", { available: balance?.available ?? "—", reserved: balance?.reserved ?? "—" })}
        </p>
        <dl className="mb-3 grid grid-cols-1 gap-3 sm:grid-cols-3" aria-label={t("walletBuckets")}>
          {[["giftBalance", balance?.gift_minor], ["purchasedBalance", balance?.purchased_minor], ["commissionBalance", balance?.commission_available_minor]].map(([label, value]) => (
            <div key={String(label)} className="rounded-control border border-hairline p-3">
              <dt className="text-sm text-ink-secondary">{t(String(label))}</dt>
              <dd className="mt-1 font-mono tabular-nums">{formatUsdMinor(value)}</dd>
            </div>
          ))}
        </dl>
        <p className="mb-4 text-sm text-ink-secondary">{t("walletBucketsDetail")}</p>
        <ListResourceView
          name="payments"
          snapshot={methodsList.snapshot}
          emptyTitle={t("payOfflineTitle")}
          emptyDetail={help || t("payOfflineDetail")}
          onRetry={() => void methodsList.reload()}
        >
          <>
            <p className="th-eyebrow mb-3 text-ink-mute">{t("stepSelect")}</p>
            <ActionRow className="mb-4">
              {chips.map((n) => (
                <Button
                  key={n}
                  type="button"
                  size="sm"
                  variant={amount === n ? "default" : "outline"}
                  disabled={selectionLocked}
                  onClick={() => setAmount(n)}
                >
                  {formatAmountChip(payCurrency, n)}
                </Button>
              ))}
            </ActionRow>
            <ActionRow className="mb-4">
              {methods.map((method) => (
                <Button
                  key={method.adapter}
                  type="button"
                  variant={adapter === method.adapter ? "default" : "outline"}
                  style={adapter === method.adapter ? { background: method.brand_color, borderColor: method.brand_color } : { color: method.brand_color, borderColor: method.brand_color }}
                  disabled={selectionLocked}
                  onClick={() => setAdapter(method.adapter)}
                >
                  {method.display_name || method.name || method.adapter}
                  {method.sandbox ? " · SANDBOX" : ""}
                </Button>
              ))}
            </ActionRow>
            {selected && !selected.auto_renew_supported ? (
              <p className="mb-3 rounded-stamp bg-canvas px-3 py-2 text-sm text-ink-secondary">{t("payNoAutoRenew")}</p>
            ) : null}
            {selected?.auto_renew_supported ? (
              <p className="mb-3 rounded-stamp bg-canvas px-3 py-2 text-sm text-ink-secondary">{t("payAutoRenew")}</p>
            ) : null}
          </>
        </ListResourceView>
      </section>
      {methodsUsable || showOrderSummary ? <aside
        className="h-fit min-w-0 rounded-card border border-hairline bg-canvas-raised p-6 lg:col-start-2 lg:row-span-2 lg:row-start-1 lg:sticky lg:top-24"
        data-testid="quote-summary"
        data-quote-phase={showOrderSummary ? "order" : quoteSnap.phase}
      >
        <p className="th-eyebrow mb-3 text-ink-mute">{showOrderSummary ? t("orderSummary") : t("stepQuote")}</p>
        {quoteSnap.phase === "loading" && !showOrderSummary ? (
          <p className="mb-3 text-sm text-ink-secondary">{t("quoteCalculating")}</p>
        ) : null}
        {quoteSnap.phase === "error" && !showOrderSummary ? (
          <div className="mb-3">
            <p className="text-sm text-danger">{quoteSnap.message || t("quoteFailed")}</p>
            <Button type="button" variant="outline" size="sm" className="mt-2" onClick={() => setQuoteTick((n) => n + 1)} data-testid="quote-retry">
              {t("quoteRetry")}
            </Button>
          </div>
        ) : null}
        <ul className="divide-y divide-hairline text-sm">
          <li className="flex justify-between py-2">
            <span>{t("ledgerDue")}</span>
            <span className="font-mono tabular-nums">{dueText}</span>
          </li>
          <li className="flex justify-between py-2">
            <span>{t("ledgerFee")}</span>
            <span className="font-mono tabular-nums">{feeText}</span>
          </li>
          <li className="flex justify-between py-2">
            <span>{t("ledgerCredit")}</span>
            <span className="font-mono tabular-nums">{creditText}</span>
          </li>
        </ul>
      </aside> : null}
      <section className="min-w-0 rounded-card border border-hairline bg-canvas-raised p-6 lg:col-start-1">
        {methodsUsable ? (
          <>
            <p className="th-eyebrow mb-3 text-ink-mute">{t("stepPay")}</p>
            {mismatch ? (
              <p className="mb-3 text-sm text-ink-secondary">
                {t("orderQuoteMismatch", {
                  order: formatOrderDue(order),
                  quote: formatPayMinor(quoteSnap.quote?.pay_currency, quoteSnap.quote?.pay_minor),
                })}
              </p>
            ) : null}
            <Button type="button" disabled={!canPay} onClick={() => void pay()} data-testid="wallet-pay">
              {payLabel}
            </Button>
            {checkout ? <CheckoutPay checkout={checkout} onPaid={() => void refresh()} /> : null}
          </>
        ) : null}
        <div className="mt-6 grid min-w-0 gap-3">
          <div className="min-w-0">
            <Label htmlFor="wallet-redeem-code">{t("redeemCode")}</Label>
            <Input
              id="wallet-redeem-code"
              className="mt-1.5 min-w-0"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </div>
          <ActionRow className="w-full max-w-full">
            <Button type="button" variant="outline" className="shrink-0" onClick={() => void refresh()}>
              {t("refreshBalance")}
            </Button>
            <Button type="button" className="shrink-0" onClick={() => void redeem()}>
              {t("redeem")}
            </Button>
          </ActionRow>
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
    </div>
  );
}
