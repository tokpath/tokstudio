export type CheckoutOrder = {
  id?: string;
  status?: string;
  credit_minor?: number;
  fulfilled_at?: string | null;
};

export type CheckoutUiStatus = "pending" | "confirming" | "failed" | "expired" | "paid" | "refunded";

export function checkoutUiStatus(status?: string, confirming = false): CheckoutUiStatus {
  const value = (status || "pending").trim().toLowerCase();
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
  if (confirming) {
    return "confirming";
  }
  return "pending";
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
