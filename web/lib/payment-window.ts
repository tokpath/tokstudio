import type { Order } from "./contracts";
import { compareDecimal } from "./decimal";

// The named window must be opened during the click that confirms KVoucher.
// The form is submitted from this page into that window after checkout returns,
// so the current page stays in place.

const paymentWindowName = "tokpath-gkash";

export function reservePaymentWindow(): Window | null {
  return window.open("about:blank", paymentWindowName);
}

export function submitPaymentForm(target: Window | null, url: string, fields: Record<string, string>) {
  if (!target || target.closed) return false;
  const form = document.createElement("form");
  form.method = "POST";
  form.action = url;
  form.target = paymentWindowName;
  form.style.display = "none";
  for (const [name, value] of Object.entries(fields)) {
    const input = document.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.append(input);
  }
  document.body.append(form);
  form.submit();
  form.remove();
  return true;
}

export function closePaymentWindow(target: Window | null) {
  if (target && !target.closed) target.close();
}

const openedPrefix = "tokpath-gkash-opened:";

// Remember that this browser opened the provider page, so the order page can
// refresh saved payment status. The server worker checks the original channel.
export function markPaymentOpened(orderID: string) {
  try { sessionStorage.setItem(openedPrefix + orderID, "1"); } catch { /* private browsing can block storage */ }
}

export function paymentWasOpened(orderID: string) {
  try { return sessionStorage.getItem(openedPrefix + orderID) === "1"; } catch { return false; }
}

// Refresh our saved order only while it can progress here. The worker already
// checks the provider; a browser must not run a second recurring channel query.
export function shouldRefreshPayment(order: Order, paymentOpened: boolean, now = Date.now()) {
  if (order.delivered_at || ["fulfilled", "completed", "closed", "cancelled", "canceled", "refunded", "expired"].includes(order.status)) return false;
  if (["executing", "outcome_unknown", "failed", "awaiting_settlement"].includes(order.payment_state || "")) return false;
  if (compareDecimal(order.paid_money, order.amount_money) >= 0) return true;
  if (order.expires_at && new Date(order.expires_at).getTime() <= now) return false;
  const action = order.payment_action;
  if (!action || action.payment_quote && new Date(action.payment_quote.expires_at).getTime() <= now) return false;
  return action.kind !== "form" || paymentOpened;
}
