"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import QRCode from "qrcode";
import type { Stripe, StripeElements } from "@stripe/stripe-js";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";
import {
  checkoutIsOpen,
  checkoutKind,
  checkoutNeedsFulfillment,
  checkoutOrderID,
  checkoutUiStatus,
  type CheckoutOrder,
  type CheckoutPayload,
} from "@/lib/checkout";
import { formatCreditMinor } from "@/lib/payment-quote";

type Props = {
  checkout: CheckoutPayload;
  onPaid?: () => void;
};

export function CheckoutPay({ checkout, onPaid }: Props) {
  const t = useTranslations("checkout");
  const kind = checkoutKind(checkout);
  const orderID = checkoutOrderID(checkout);
  const [order, setOrder] = useState<CheckoutOrder>(checkout.order || { status: "pending" });
  const [qrSvg, setQrSvg] = useState("");
  const [busy, setBusy] = useState(false);
  const ui = checkoutUiStatus(order.status, busy);
  const open = checkoutIsOpen(ui);
  const paid = ui === "paid";
  const awaitingCredit = checkoutNeedsFulfillment(order);

  const applyOrder = useCallback(
    (next?: CheckoutOrder) => {
      if (!next?.status) return;
      setOrder((prev) => ({ ...prev, ...next }));
      if (next.status === "paid") onPaid?.();
    },
    [onPaid],
  );

  useEffect(() => {
    setOrder(checkout.order || { status: "pending" });
  }, [checkout.order, orderID]);

  useEffect(() => {
    if (kind !== "qr" || !checkout.qr_code || !open) {
      setQrSvg("");
      return;
    }
    let cancelled = false;
    void QRCode.toString(checkout.qr_code, { type: "svg", margin: 1, width: 220, color: { dark: "#141414", light: "#FFFDF8" } }).then(
      (svg) => {
        if (!cancelled) setQrSvg(svg);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [kind, checkout.qr_code, open]);

  useEffect(() => {
    if (!orderID || (paid && !awaitingCredit) || ui === "failed" || ui === "expired" || ui === "refunded") {
      return;
    }
    const timer = window.setInterval(() => {
      void fetch(`${apiBase}/v1/payments/orders/${encodeURIComponent(orderID)}`, { credentials: "include" })
        .then((res) => res.json())
        .then((body) => applyOrder(body.item as CheckoutOrder));
    }, 4000);
    return () => window.clearInterval(timer);
  }, [applyOrder, awaitingCredit, orderID, paid, ui]);

  async function syncPaid() {
    if (!orderID) return;
    setBusy(true);
    try {
      const response = await fetch(`${apiBase}/v1/payments/orders/${encodeURIComponent(orderID)}/sync`, {
        method: "POST",
        credentials: "include",
      });
      const body = await response.json();
      applyOrder(body.item as CheckoutOrder);
    } finally {
      setBusy(false);
    }
  }

  function statusCopy() {
    if (ui === "paid") {
      if (awaitingCredit) {
        return t("paidPendingCredit");
      }
      return t("paidCredited", { credit: formatCreditMinor(order.credit_minor) });
    }
    return t(ui);
  }

  return (
    <div className="mt-4 rounded-card border border-hairline bg-canvas p-4" data-checkout-status={ui}>
      <p className="th-eyebrow text-ink-mute">{t("orderEyebrow")}</p>
      <p className="mt-1 font-mono text-sm tabular-nums text-ink">{orderID || "—"}</p>
      <p className="mt-1 text-sm text-ink-secondary">{statusCopy()}</p>
      {kind === "sandbox" && open ? <p className="mt-3 text-sm text-ink-secondary">{t("sandboxHint")}</p> : null}
      {kind === "qr" && open ? (
        <div className="mt-3">
          <p className="mb-2 text-sm text-ink-secondary">{t("scanQr")}</p>
          {qrSvg ? (
            <div
              role="img"
              aria-label={t("scanQr")}
              className="inline-block h-[220px] w-[220px] overflow-hidden rounded-stamp border border-hairline bg-canvas-raised p-2 [&_svg]:h-full [&_svg]:w-full"
              dangerouslySetInnerHTML={{ __html: qrSvg }}
            />
          ) : (
            <p className="break-all font-mono text-xs text-ink-mute">{checkout.qr_code}</p>
          )}
        </div>
      ) : null}
      {kind === "redirect" && open && checkout.redirect_url ? (
        <Button className="mt-3" asChild>
          <a href={checkout.redirect_url}>{t("redirect")}</a>
        </Button>
      ) : null}
      {kind === "element" && open && checkout.client_secret && checkout.publishable_key ? (
        <StripeElementPay
          publishableKey={checkout.publishable_key}
          clientSecret={checkout.client_secret}
          onPaid={() => applyOrder({ status: "paid" })}
        />
      ) : null}
      {open && kind !== "sandbox" ? (
        <Button type="button" variant="outline" className="mt-3" disabled={busy} onClick={() => void syncPaid()}>
          {busy ? t("checking") : t("paidCheck")}
        </Button>
      ) : null}
    </div>
  );
}

function StripeElementPay({
  publishableKey,
  clientSecret,
  onPaid,
}: {
  publishableKey: string;
  clientSecret: string;
  onPaid: () => void;
}) {
  const t = useTranslations("checkout");
  const hostRef = useRef<HTMLDivElement>(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");
  const stripeRef = useRef<Stripe | null>(null);
  const elementsRef = useRef<StripeElements | null>(null);

  useEffect(() => {
    let cancelled = false;
    let payment: { mount: (el: HTMLElement) => void; unmount: () => void } | null = null;
    void (async () => {
      const { loadStripe } = await import("@stripe/stripe-js");
      const stripe = await loadStripe(publishableKey);
      if (!stripe || cancelled || !hostRef.current) return;
      const elements = stripe.elements({ clientSecret });
      payment = elements.create("payment");
      payment.mount(hostRef.current);
      stripeRef.current = stripe;
      elementsRef.current = elements;
      if (!cancelled) setReady(true);
    })();
    return () => {
      cancelled = true;
      payment?.unmount();
    };
  }, [publishableKey, clientSecret]);

  async function confirm() {
    if (!stripeRef.current || !elementsRef.current) return;
    setError("");
    const result = await stripeRef.current.confirmPayment({
      elements: elementsRef.current,
      confirmParams: { return_url: window.location.href },
      redirect: "if_required",
    });
    if (result.error?.message) {
      setError(result.error.message);
      return;
    }
    onPaid();
  }

  return (
    <div className="mt-3">
      <p className="mb-2 text-sm text-ink-secondary">{t("stripeNeed")}</p>
      <div ref={hostRef} className="min-h-[120px]" />
      <Button type="button" className="mt-3" disabled={!ready} onClick={() => void confirm()}>
        {t("stripePay")}
      </Button>
      {error ? <p className="mt-2 text-sm text-danger">{error}</p> : null}
    </div>
  );
}
