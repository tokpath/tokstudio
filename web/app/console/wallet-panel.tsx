"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { restoreWalletOperation, type WalletOperation } from "@/lib/wallet-operation";
import { walletReturnPath, walletTabHref, purchaseIsResolved } from "@/lib/wallet-context";
import { notifyWalletChanged } from "@/lib/wallet-events";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ActionRow } from "@/components/console/action-row";
import { ListResourceView } from "@/components/console/list-resource-view";
import { CheckoutPay } from "@/components/checkout-pay";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import type { CheckoutOrder, CheckoutPayload } from "@/lib/checkout";
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
  commission_recovery_minor?: number;
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
  const intent = useTranslations("publicExperience");
  const search = useSearchParams();
  const returnPath = walletReturnPath(search.get("next"));
  const viewer=useViewer();
  const brand=useBrand();
  const operationText=useTranslations("walletOperation");
  const tc = useTranslations("common");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [code, setCode] = useState("");
  const te = useTranslations("entitlements");
  const w = useTranslations("walletExperience");
  const [balanceError,setBalanceError]=useState("");
  const [redeeming,setRedeeming]=useState(false);
  const redeemBusy=useRef(false);
  const balanceGeneration=useRef(0);
  const [message, setMessage] = useState("");
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
  const operation=useRef<WalletOperation|null>(null);
  const busy=useRef(false);
  const [pending,setPending]=useState<WalletOperation|null>(null);
  const storageKey=`tokenhub_wallet_purchase:${viewer.userId || ""}:${brand?.id || ""}`;
  const scopeRef=useRef(storageKey);scopeRef.current=storageKey;
  const methodsList = useListResource<Method>({
    enabled:!viewer.loading,
    queryKey:storageKey,
    load: async () => {
      try {
        const response = await fetch(`${apiBase}/v1/payments/checkout`, { credentials: "include" });
        const body = await response.json().catch(() => ({}));
        if (!response.ok) {
          return { ok: false, status: response.status, items: [], message: body.error?.message, code: body.error?.code };
        }
        const item = body.item || {};
        if(scopeRef.current!==storageKey)return {ok:true,status:response.status,items:[]};
        const nextMethods: Method[] = item.methods || [];
        setHelp(item.help_text || "");
        const amounts: number[] = item.settings?.quick_amounts || [100, 300, 500, 1000];
        setChips(amounts);
        if (!operation.current && amounts[0]) setAmount(amounts[0]);
        if (!operation.current && nextMethods[0]) setAdapter(nextMethods[0].adapter);
        return { ok: true, status: response.status, items: nextMethods };
      } catch {
        return { ok: false, network: true, items: [] };
      }
    },
  });
  const methods = methodsList.snapshot.items;

  async function refresh(manual=false) {
    const generation=++balanceGeneration.current;
    try {
      const response=await fetch(`${apiBase}/v1/me/balance`,{credentials:"include"});
      const body=await response.json();
      if(scopeRef.current!==storageKey || generation!==balanceGeneration.current)return;
      if(!response.ok || !body.balance)throw new Error();
      setBalance(body.balance);setBalanceError("");
      if(manual)setMessage(t("walletRefreshed"));
    } catch {
      if(scopeRef.current===storageKey && generation===balanceGeneration.current)setBalanceError(w("balanceError"));
    }
  }
  function changed(){notifyWalletChanged({userId:viewer.userId || "",brandId:brand?.id || ""});void refresh();}
  useEffect(()=>{if(!viewer.loading && viewer.userId)void refresh();},[storageKey,viewer.loading]); // eslint-disable-line react-hooks/exhaustive-deps

  function saveOperation(op:WalletOperation){operation.current=op;setPending(op);try{sessionStorage.setItem(storageKey,JSON.stringify(op));}catch{/* private mode */}}
  function clearOperation(){operation.current=null;setPending(null);setCreateUnknown(false);try{sessionStorage.removeItem(storageKey);}catch{/* private mode */}}
  async function recoverOperation(op:WalletOperation){
    try{
      const response=await fetch(`${apiBase}/v1/me/wallet-purchases/${encodeURIComponent(op.id)}`,{credentials:"include"});
      const body=await response.json();
      if(scopeRef.current!==storageKey)return;
      if(!response.ok || !body.item?.id)return;
      const item=body.item;
      saveOperation({...op,orderId:item.id});
      setCheckout({order:item,sandbox:false});setCreateUnknown(false);
      if(purchaseIsResolved(item))clearOperation();
      if(item.status==="paid"){setMessage(w("paymentConfirmed"));changed();}
    }catch{/* An unknown lookup must retain the original operation and known order. */}
  }
  useEffect(()=>{
    busy.current=false;setCreating(false);operation.current=null;setPending(null);setCheckout(null);setBalance(null);setBalanceError("");setMessage("");setCode("");redeemBusy.current=false;setRedeeming(false);setCreateUnknown(false);
    try{const saved=restoreWalletOperation(sessionStorage.getItem(storageKey));if(saved){saveOperation(saved);setAdapter(saved.adapter);setAmount(saved.payMajor);setCreateUnknown(true);void recoverOperation(saved);}}catch{/* private mode */}
    // Scope changes always invalidate old responses and drafts from another account.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  },[storageKey]);

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
  }, [adapter, amount, quoteTick, t, storageKey]);

  async function redeem() {
    if(redeemBusy.current || !code.trim())return;
    redeemBusy.current=true;setRedeeming(true);
    try {
      const response=await fetch(`${apiBase}/v1/topups/redeem`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify({code:code.trim()})});
      const body=await response.json();
      if(scopeRef.current!==storageKey)return;
      if(response.ok){setMessage(t("redeemOk",{amount:formatUsdMinor(body.item?.amount_minor)}));setCode("");changed();}
      else setMessage(body.error?.message || t("redeemFail"));
    } catch {if(scopeRef.current===storageKey)setMessage(t("redeemFail"));}
    finally {if(scopeRef.current===storageKey){redeemBusy.current=false;setRedeeming(false);}}
  }

  async function pay() {
    if (busy.current) {
      return;
    }
    if (operation.current) {
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
    if(busy.current)return;
    busy.current=true;
    const op=operation.current || {id:crypto.randomUUID(),adapter,payMajor:amount};
    saveOperation(op);
    setCreating(true);
    try {
      const response = await fetch(`${apiBase}/v1/payments/orders`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json", "Idempotency-Key": op.id },
        body: JSON.stringify({ adapter:op.adapter, pay_major:op.payMajor }),
      });
      const body = await response.json().catch(() => ({}));
      if(scopeRef.current!==storageKey)return;
      if (response.ok && body.checkout?.order?.id) {
        const next = (body.checkout || null) as CheckoutPayload | null;
        if (next?.order && !next.order.adapter) {
          next.order.adapter = next.adapter;
        }
        setCheckout(next);
        saveOperation({...op,orderId:body.checkout.order.id});
        setCreateUnknown(false);
        setMessage(t("payOrderOk", { id: body.checkout?.order?.id }));
        if(purchaseIsResolved(body.checkout.order))clearOperation();
        if(body.checkout.order.status==="paid"){setMessage(w("paymentConfirmed"));changed();}
      } else {
        setCreateUnknown(true);
        setMessage(body.error?.message || t("payOrderFail"));
        await recoverOperation(op);
      }
    } catch {
      if(scopeRef.current===storageKey){setCreateUnknown(true);setMessage(t("payCreateUnconfirmed"));await recoverOperation(op);}
    } finally {
      if(scopeRef.current===storageKey){busy.current=false;setCreating(false);}
    }
  }

  function resolved(fact:CheckoutOrder){
    if(scopeRef.current!==storageKey || fact.id!==checkout?.order?.id)return;
    setCheckout(previous=>previous ? {...previous,order:{...previous.order,...fact}} : previous);
    if(purchaseIsResolved(fact) && operation.current?.orderId===fact.id)clearOperation();
  }
  const selected = methods.find((m) => m.adapter === adapter);
  const payCurrency = selected?.pay_currency || quoteSnap.quote?.pay_currency;
  const order = checkout?.order;
  const showOrderSummary = !!order && !purchaseIsResolved(order) && orderMatchesSelection(order,adapter,amount);
  const mismatch = Boolean(order && !showOrderSummary && quoteMatchesSelection(quoteSnap.quote, adapter, amount));
  const selectionLocked = creating || !!pending;
  const canPay =
    !!pending || (quoteSnap.phase === "ready" && quoteMatchesSelection(quoteSnap.quote, adapter, amount) && !creating);
  const payLabel = creating
    ? t("creatingOrder")
    : pending
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
        {pending && (!checkout || createUnknown || (checkout.order?.status==="pending" && !checkout.sandbox && !checkout.qr_code && !checkout.redirect_url && !checkout.client_secret)) && <div className="mb-5 rounded-control border border-hairline p-4"><p role="status">{operationText("resume")}</p><p className="my-2 text-sm">{operationText("original",{amount:pending.payMajor,method:pending.adapter})}</p>{pending.orderId && <p className="mb-2 break-all font-mono text-sm">{operationText("order",{id:pending.orderId})}</p>}<Button disabled={creating} variant="outline" onClick={()=>void submitOrder()}>{t("recoverCreate")}</Button></div>}
        <p className="mb-4 text-sm text-ink-secondary">
          {t("walletMeta", { available: balance?.available ?? "—", reserved: balance?.reserved ?? "—" })}
        </p>
        <dl className="mb-3 grid grid-cols-1 gap-3 sm:grid-cols-2" aria-label={t("walletBuckets")}>
          {[["giftBalance", balance?.gift_minor], ["purchasedBalance", balance?.purchased_minor]].map(([label, value]) => (
            <div key={String(label)} className="rounded-control border border-hairline p-3">
              <dt className="text-sm text-ink-secondary">{t(String(label))}</dt>
              <dd className="mt-1 font-mono tabular-nums">{formatUsdMinor(value)}</dd>
            </div>
          ))}
        </dl>
        {balanceError && <p role="alert" className="mb-3 text-sm text-danger">{balanceError}</p>}
        <Link className="mb-4 inline-block text-sm text-brand-emphasis underline" href="/app/referral">{w("referralLink")}</Link>
        {returnPath && <Link className="mb-4 ml-4 inline-block text-sm text-brand-emphasis underline" href={returnPath}>{intent("returnTask")}</Link>}
        <p className="mb-4 text-sm text-ink-secondary"><Link className="text-brand-emphasis underline" href={walletTabHref(search.toString(),"plans")}>{te("walletLink")}</Link></p>
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
            {(!checkout || !pending || createUnknown) && <Button type="button" disabled={!canPay} onClick={() => void pay()} data-testid="wallet-pay">
              {payLabel}
            </Button>}

          </>
        ) : null}
        {checkout && <CheckoutPay checkout={checkout} onResolved={resolved} onPaid={()=>{if(scopeRef.current===storageKey){setMessage(w("paymentConfirmed"));changed();}}} />}
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
            <Button type="button" variant="outline" className="shrink-0" onClick={() => {void refresh(true);notifyWalletChanged({userId:viewer.userId || "",brandId:brand?.id || ""});}}>
              {t("refreshBalance")}
            </Button>
            <Button type="button" className="shrink-0" disabled={redeeming || !code.trim()} onClick={() => void redeem()}>
              {t("redeem")}
            </Button>
          </ActionRow>
        </div>
        {message && <p role="status" className="mt-3 text-sm text-ink-secondary">{message}</p>}
      </section>
    </div>
  );
}
