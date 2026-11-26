import { newActionKey, request } from "./api-client";
import { GkashOpenRequestSchema, GkashOpenResultSchema, type Order } from "./contracts";
import { paymentDeadlinePassed } from "./customer-order-status";
import { compareDecimal, safeAssetURL } from "./decimal";

export type PaymentWindow = { handle: Window; name: string };
type PaymentOrder = Pick<Order, "id" | "payment_action" | "expires_at"> & { status?: string };
type IssuedPayment = { generation: number; target?: PaymentWindow };
export type PaymentWindowState = "unopened" | "open" | "opened";
const issuedPayments = new Map<string, IssuedPayment>();
const openingOrders = new Set<string>();
const openedPrefix = "tokpath-gkash-opened:";
const paymentWindowChanged = "tokpath-gkash-window-changed";

function isWindowOpen(target?: PaymentWindow | null): target is PaymentWindow {
  try { return !!target && !target.handle.closed; }
  catch { return false; }
}

export function reservePaymentWindow(): PaymentWindow | null {
  for (const payment of issuedPayments.values()) {
    if (!isWindowOpen(payment.target)) payment.target = undefined;
  }
  // Reserve a distinct target during the click, before requesting the form.
  const name = `tokpath-gkash-${newActionKey()}`;
  try {
    const handle = window.open("about:blank", name);
    return handle ? { handle, name } : null;
  } catch {
    return null;
  }
}

function submitPaymentForm(target: PaymentWindow, url: string, fields: Record<string, string>) {
  const action = safeAssetURL(url);
  if (!action || !Object.keys(fields).length) throw new Error("支付链接暂不可用，请刷新原订单或联系经营方。");
  if (!isWindowOpen(target)) throw new Error("付款窗口未能打开。请允许弹出窗口后从原订单继续。");
  const form = document.createElement("form");
  form.method = "POST";
  form.action = action;
  form.target = target.name;
  form.style.display = "none";
  for (const [name, value] of Object.entries(fields)) {
    const input = document.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.append(input);
  }
  document.body.append(form);
  try { HTMLFormElement.prototype.submit.call(form); }
  finally { form.remove(); }
}

export function closePaymentWindow(target: PaymentWindow | null) {
  if (isWindowOpen(target)) {
    try { target.handle.close(); }
    catch { /* The browser has withdrawn access to the reserved window. */ }
  }
}

export function focusPaymentWindow(orderID: string) {
  const payment = issuedPayments.get(orderID);
  if (!payment?.target) return false;
  const target = payment.target;
  if (isWindowOpen(target)) {
    try {
      target.handle.focus();
      return true;
    } catch { /* The browser has withdrawn access to the payment window. */ }
  }
  payment.target = undefined;
  window.dispatchEvent(new Event(paymentWindowChanged));
  return false;
}

export function customerGkashOpenPath(orderID: string) {
  return `/api/v1/me/orders/${encodeURIComponent(orderID)}/payment/open`;
}

// Only this click's server response can supply a signed form.
export async function openGkashPayment(order: PaymentOrder, path: string, reserved?: PaymentWindow | null) {
  const orderID = order.id;
  if (paymentDeadlinePassed(order)) {
    closePaymentWindow(reserved ?? null);
    throw new Error("付款期限已过。请新建订单付款。");
  }
  if (focusPaymentWindow(orderID) || openingOrders.has(orderID)) {
    closePaymentWindow(reserved ?? null);
    return;
  }
  const target = reserved === undefined ? reservePaymentWindow() : reserved;
  if (!isWindowOpen(target)) throw new Error("付款窗口未能打开。请允许弹出窗口后从原订单继续。");
  openingOrders.add(orderID);
  try {
    const expected = Math.max(order.payment_action?.gkash_generation ?? 0, issuedPayments.get(orderID)?.generation ?? 0);
    const body = GkashOpenRequestSchema.parse({ expected_generation: expected });
    // Reopening may wait for both the original payment lookup and a new signature.
    const result = await request(path, GkashOpenResultSchema, { method: "POST", body, actionKey: newActionKey(), timeoutMs: 45_000 });
    if (result.id !== orderID) throw new Error("支付链接暂不可用，请刷新原订单或联系经营方。");
    if (paymentDeadlinePassed(order)) throw new Error("付款期限已过。请新建订单付款。");
    if (result.status === "payment_already_opened") {
      markPaymentOpened(orderID);
      return result.message || "原 Gkash 付款页已打开过。请核实原付款状态。";
    }
    if (result.gkash_generation <= expected) throw new Error("支付链接暂不可用，请刷新原订单或联系经营方。");
    const payment: IssuedPayment = { generation: result.gkash_generation };
    issuedPayments.set(orderID, payment);
    try {
      submitPaymentForm(target, result.payment_action.url, result.payment_action.fields);
      payment.target = target;
    } finally {
      markPaymentOpened(orderID);
    }
  } finally {
    openingOrders.delete(orderID);
    if (issuedPayments.get(orderID)?.target !== target) closePaymentWindow(target);
  }
}

// Remember the checkout was issued so this page can refresh its saved status.
// The server worker checks the original channel.
export function markPaymentOpened(orderID: string) {
  try { sessionStorage.setItem(openedPrefix + orderID, "1"); } catch { /* private browsing can block storage */ }
  window.dispatchEvent(new Event(paymentWindowChanged));
}

export function paymentWasOpened(orderID: string) {
  if (issuedPayments.has(orderID)) return true;
  try { return sessionStorage.getItem(openedPrefix + orderID) === "1"; } catch { return false; }
}

export function paymentWindowState(order: PaymentOrder): PaymentWindowState {
  const target = issuedPayments.get(order.id)?.target;
  if (isWindowOpen(target)) return "open";
  return order.payment_action?.gkash_opened || paymentWasOpened(order.id) ? "opened" : "unopened";
}

export function watchPaymentWindow(listener: () => void) {
  window.addEventListener(paymentWindowChanged, listener);
  window.addEventListener("focus", listener);
  return () => {
    window.removeEventListener(paymentWindowChanged, listener);
    window.removeEventListener("focus", listener);
  };
}

// Refresh our saved order only while it can progress here. The worker already
// checks the provider; a browser must not run a second recurring channel query.
export function shouldRefreshPayment(order: Order, paymentOpened: boolean, now = Date.now()) {
  if (order.delivered_at || ["fulfilled", "completed", "closed", "cancelled", "canceled", "refunded", "expired"].includes(order.status)) return false;
  if (["executing", "outcome_unknown", "failed", "closed", "awaiting_settlement"].includes(order.payment_state || "")) return false;
  if (compareDecimal(order.paid_money, order.amount_money) >= 0) return true;
  const action = order.payment_action;
  if (!action || paymentDeadlinePassed(order, now)) return false;
  return action.kind !== "form" || paymentOpened;
}
