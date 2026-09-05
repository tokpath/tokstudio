"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ActionRow } from "@/components/console/action-row";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { apiBase } from "@/lib/api";

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

type Quote = {
  pay_major?: number;
  pay_currency?: string;
  pay_minor?: number;
  fee_minor?: number;
  wallet_minor?: number;
  credit_minor?: number;
};

export default function WalletPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [code, setCode] = useState("");
  const [message, setMessage] = useState(t("walletHint"));
  const [methods, setMethods] = useState<Method[]>([]);
  const [help, setHelp] = useState("");
  const [chips, setChips] = useState<number[]>([100, 300, 500, 1000]);
  const [amount, setAmount] = useState(100);
  const [adapter, setAdapter] = useState("");
  const [quote, setQuote] = useState<Quote | null>(null);

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

  async function loadCheckout() {
    const response = await fetch(`${apiBase}/v1/payments/checkout`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      return;
    }
    const item = body.item || {};
    const list: Method[] = item.methods || [];
    setMethods(list);
    setHelp(item.help_text || "");
    const amounts: number[] = item.settings?.quick_amounts || [100, 300, 500, 1000];
    setChips(amounts);
    if (amounts[0]) setAmount(amounts[0]);
    if (list[0]) setAdapter(list[0].adapter);
  }

  async function loadQuote(nextAdapter: string, nextAmount: number) {
    if (!nextAdapter || nextAmount <= 0) {
      setQuote(null);
      return;
    }
    const response = await fetch(
      `${apiBase}/v1/payments/quote?adapter=${encodeURIComponent(nextAdapter)}&pay_major=${nextAmount}`,
      { credentials: "include" },
    );
    const body = await response.json();
    if (response.ok) setQuote(body.item);
  }

  useEffect(() => {
    void refresh();
    void loadCheckout();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void loadQuote(adapter, amount);
  }, [adapter, amount]);

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
    const response = await fetch(`${apiBase}/v1/payments/orders`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ adapter, pay_major: amount }),
    });
    const body = await response.json();
    setMessage(
      response.ok
        ? t("payOrderOk", { id: body.checkout?.order?.id })
        : body.error?.message || t("payOrderFail"),
    );
  }

  const selected = methods.find((m) => m.adapter === adapter);
  const credit = ((quote?.credit_minor || 0) / 1_000_000).toFixed(2);
  const payLabel =
    quote?.pay_currency === "CNY"
      ? t("payCny", { amount, credit })
      : t("payUsd", { amount, credit });

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="mb-4 text-sm text-ink-secondary">
          {t("walletMeta", { available: balance?.available ?? "—", reserved: balance?.reserved ?? "0" })}
        </p>
        {methods.length === 0 ? (
          <EmptyLedger title={t("payOfflineTitle")} detail={help || t("payOfflineDetail")} />
        ) : (
          <>
            <ActionRow className="mb-4">
              {chips.map((n) => (
                <Button key={n} type="button" size="sm" variant={amount === n ? "default" : "outline"} onClick={() => setAmount(n)}>
                  {n}
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
            <Button type="button" onClick={() => void pay()}>
              {payLabel}
            </Button>
          </>
        )}
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
      <aside className="h-fit rounded-card border border-hairline bg-canvas-raised p-6 lg:sticky lg:top-24">
        <p className="th-eyebrow mb-3 text-ink-mute">{t("ledgerEyebrow")}</p>
        <ul className="divide-y divide-hairline text-sm">
          <li className="flex justify-between py-2">
            <span>{t("ledgerDue")}</span>
            <span className="font-mono tabular-nums">
              {quote?.pay_currency === "CNY" ? `¥${((quote.pay_minor || 0) / 100).toFixed(2)}` : `$${((quote?.pay_minor || 0) / 1_000_000).toFixed(2)}`}
            </span>
          </li>
          <li className="flex justify-between py-2">
            <span>{t("ledgerFee")}</span>
            <span className="font-mono tabular-nums">
              {quote?.pay_currency === "CNY" ? `¥${((quote.fee_minor || 0) / 100).toFixed(2)}` : `$${((quote?.fee_minor || 0) / 1_000_000).toFixed(2)}`}
            </span>
          </li>
          <li className="flex justify-between py-2">
            <span>{t("ledgerCredit")}</span>
            <span className="font-mono tabular-nums">${((quote?.credit_minor || 0) / 1_000_000).toFixed(2)}</span>
          </li>
        </ul>
      </aside>
    </div>
  );
}
