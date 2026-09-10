export type CheckoutOrder = {
  id?: string;
  status?: string;
};

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
