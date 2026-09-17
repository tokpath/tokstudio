import { formatCreditMinor, formatPayMinor } from "@/lib/payment-quote";

export type CheckoutOrder = {
  id?: string;
  status?: string;
  adapter?: string;
  currency?: string;
  amount_minor?: number;
  credit_minor?: number;
  fulfilled_at?: string | null;
};

export type CheckoutUiStatus =
  | "pending"
  | "confirming"
  | "failed"
  | "expired"
  | "paid"
  | "refunded"
  | "partially_refunded"
  | "unknown";

export function checkoutUiStatus(status?: string, confirming = false): CheckoutUiStatus {
  const value = (status || "").trim().toLowerCase();
  if (value === "paid") {
    return "paid";
  }
  if (value === "failed") {
    return "failed";
  }
  if (value === "expired") {
    return "expired";
  }
  if (value === "refunded") {
    return "refunded";
  }
  if (value === "partially_refunded" || value === "partial_refund") {
    return "partially_refunded";
  }
  if (value === "processing" || value === "requires_action") {
    return "confirming";
  }
  if (confirming) {
    return "confirming";
  }
  if (value === "pending" || value === "") {
    return "pending";
  }
  return "unknown";
}

export function checkoutNeedsFulfillment(order?: CheckoutOrder | null): boolean {
  if ((order?.status || "").trim().toLowerCase() !== "paid") {
    return false;
  }
  return !order?.fulfilled_at;
}

export function checkoutIsOpen(ui: CheckoutUiStatus): boolean {
  return ui === "pending" || ui === "confirming";
}

export function stripeClientOutcome(result: { error?: { message?: string } | null }): "error" | "confirming" {
  if (result.error?.message) {
    return "error";
  }
  return "confirming";
}

export function orderMatchesSelection(order: CheckoutOrder | null | undefined, adapter: string, amount: number): boolean {
  if (!order || !adapter || amount <= 0) {
    return false;
  }
  if ((order.adapter || "").trim() !== adapter) {
    return false;
  }
  const minor = Number(order.amount_minor) || 0;
  if ((order.currency || "").toUpperCase() === "CNY") {
    return minor === amount * 100;
  }
  return minor === amount * 1_000_000;
}

export function formatOrderDue(order?: CheckoutOrder | null): string {
  if (!order || order.amount_minor == null) {
    return "—";
  }
  return formatPayMinor(order.currency, order.amount_minor);
}

export function formatOrderCredit(order?: CheckoutOrder | null): string {
  return formatCreditMinor(order?.credit_minor);
}

export type CheckoutPayload = {
  order?: CheckoutOrder;
  adapter?: string;
  sandbox?: boolean;
  mode?: string;
  qr_code?: string;
  redirect_url?: string;
  client_secret?: string;
  publishable_key?: string;
  webhook_url?: string;
};

export type CheckoutKind = "qr" | "element" | "redirect" | "sandbox";

export function checkoutKind(checkout: CheckoutPayload | null | undefined): CheckoutKind {
  if (!checkout) {
    return "sandbox";
  }
  if (checkout.qr_code) {
    return "qr";
  }
  if (checkout.client_secret && checkout.publishable_key) {
    return "element";
  }
  if (checkout.redirect_url) {
    return "redirect";
  }
  return "sandbox";
}

export function checkoutOrderID(checkout: CheckoutPayload | null | undefined): string {
  return checkout?.order?.id?.trim() || "";
}

/** Drop a get/sync body when the request was for another order than the one now on screen. */
export function checkoutResponseBelongsToOrder(
  requestedID: string,
  currentID: string,
  item?: CheckoutOrder | null,
): boolean {
  const requested = requestedID.trim();
  const current = currentID.trim();
  if (!requested || requested !== current) {
    return false;
  }
  const itemID = item?.id?.trim();
  if (itemID && itemID !== requested) {
    return false;
  }
  return Boolean(item?.status);
}
