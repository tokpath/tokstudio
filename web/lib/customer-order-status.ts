import type { Order } from "./contracts";
import { compareDecimal } from "./decimal";

type CustomerOrderPayment = Partial<Pick<Order,
  "status" | "paid_money" | "amount_money" | "has_verified_receipt" | "payment_state" | "expires_at" | "delivered_at"
>>;

export const customerOrderStatusLabels = { expired: "已过期" };

function deadlinePassed(value: string | null | undefined, now: number) {
  return Date.parse(value ?? "") <= now;
}

export function paymentDeadlinePassed(order: Partial<Pick<Order, "status" | "expires_at" | "payment_action">>, now = Date.now()) {
  return order.status === "expired" || deadlinePassed(order.expires_at, now) || deadlinePassed(order.payment_action?.payment_quote?.expires_at, now);
}

export function customerOrderStatus(order: CustomerOrderPayment, now: number) {
  const status = order.status || "";
  const paidMoney = order.paid_money;
  const hasAllocatedPayment = paidMoney !== undefined && compareDecimal(paidMoney, "0") > 0;
  const hasReceipt = order.has_verified_receipt || hasAllocatedPayment || order.payment_state === "awaiting_settlement";
  const awaitingPayment = ["pending", "pending_payment", "awaiting_payment"].includes(status);
  if (order.delivered_at || !(awaitingPayment || status === "expired" && hasReceipt)) return status;
  if (order.payment_state === "awaiting_settlement") return "awaiting_settlement";
  if (hasAllocatedPayment) {
    return order.amount_money !== undefined && compareDecimal(paidMoney, order.amount_money) >= 0 ? "paid_pending_delivery" : "partially_paid";
  }
  if (order.has_verified_receipt) return "pending_verification";
  return paidMoney !== undefined && deadlinePassed(order.expires_at, now) ? "expired" : status;
}
