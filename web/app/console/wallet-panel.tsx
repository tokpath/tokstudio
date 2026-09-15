"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ActionRow } from "@/components/console/action-row";
import { ListResourceView } from "@/components/console/list-resource-view";
import { CheckoutPay } from "@/components/checkout-pay";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import type { CheckoutPayload } from "@/lib/checkout";
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
  const [checkout, setCheckout] = useState<CheckoutPayload | null>(null);
  const quoteGen = useRef(0);
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
    setCheckout(null);
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
  }, [adapter, amount, t]);

  async function redeem() {
    const response = await fetch(`${apiBase}/v1/topups/redeem`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });
    const body = await response.json();
    setMessage(response.ok ? t("redeemOk", { amount: body.item?.amount_minor }) : body.error?.message || t("redeemFail"));
    if (response.ok) await refresh();
  }

  async function pay() {
    const quote = quoteSnap.quote;
    if (creating || !quoteMatchesSelection(quote, adapter, amount)) {
      return;
    }
    setCreating(true);
    try {
      const response = await fetch(`${apiBase}/v1/payments/orders`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ adapter, pay_major: amount }),
      });
      const body = await response.json().catch(() => ({}));
      if (response.ok) {
        setCheckout(body.checkout || null);
        setMessage(t("payOrderOk", { id: body.checkout?.order?.id }));
      } else {
        setCheckout(null);
        setMessage(body.error?.message || t("payOrderFail"));
      }
    } finally {
      setCreating(false);
    }
  }

  const selected = methods.find((m) => m.adapter === adapter);
  const payCurrency = selected?.pay_currency || quoteSnap.quote?.pay_currency;
  const canPay = quoteSnap.phase === "ready" && quoteMatchesSelection(quoteSnap.quote, adapter, amount) && !creating;
  const payLabel = creating
    ? t("creatingOrder")
    : quoteSnap.phase === "loading"
      ? t("quoteCalculating")
      : canPay
        ? t("payNow", { money: formatPayMinor(quoteSnap.quote?.pay_currency, quoteSnap.quote?.pay_minor) })
        : t("quoteUnavailable");

  const dueText =
    quoteSnap.phase === "ready" && quoteSnap.quote
      ? formatPayMinor(quoteSnap.quote.pay_currency, quoteSnap.quote.pay_minor)
      : "—";
  const feeText =
    quoteSnap.phase === "ready" && quoteSnap.quote
      ? formatPayMinor(quoteSnap.quote.pay_currency, quoteSnap.quote.fee_minor)
      : "—";
  const creditText =
    quoteSnap.phase === "ready" && quoteSnap.quote ? formatCreditMinor(quoteSnap.quote.credit_minor) : "—";

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="mb-4 text-sm text-ink-secondary">
          {t("walletMeta", { available: balance?.available ?? "—", reserved: balance?.reserved ?? "0" })}
        </p>
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
                <Button key={n} type="button" size="sm" variant={amount === n ? "default" : "outline"} onClick={() => setAmount(n)}>
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
            <p className="th-eyebrow mb-3 text-ink-mute">{t("stepPay")}</p>
            <Button type="button" disabled={!canPay} onClick={() => void pay()} data-testid="wallet-pay">
              {payLabel}
            </Button>
            {checkout ? <CheckoutPay checkout={checkout} onPaid={() => void refresh()} /> : null}
          </>
        </ListResourceView>
        <ActionRow className="mt-6 w-full flex-nowrap gap-3">
          <Button type="button" variant="outline" className="shrink-0" onClick={() => void refresh()}>
            {t("refreshBalance")}
          </Button>
          <Input
            aria-label={t("redeemCode")}
            className="min-w-0 max-w-xs flex-1"
            value={code}
            onChange={(e) => setCode(e.target.value)}
          />
          <Button type="button" className="shrink-0" onClick={() => void redeem()}>
            {t("redeem")}
          </Button>
        </ActionRow>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      <aside className="h-fit rounded-card border border-hairline bg-canvas-raised p-6 lg:sticky lg:top-24" data-testid="quote-summary" data-quote-phase={quoteSnap.phase}>
        <p className="th-eyebrow mb-3 text-ink-mute">{t("stepQuote")}</p>
        {quoteSnap.phase === "loading" ? (
          <p className="mb-3 text-sm text-ink-secondary">{t("quoteCalculating")}</p>
        ) : null}
        {quoteSnap.phase === "error" ? (
          <p className="mb-3 text-sm text-danger">{quoteSnap.message || t("quoteFailed")}</p>
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
      </aside>
    </div>
  );
}
