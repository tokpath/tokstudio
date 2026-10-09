"use client";
import Link from "next/link";
import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction, canWrite } from "@/lib/rbac";

export function PaymentsNav({ scope, active }: { scope: "admin" | "channel"; active: "orders" | "lanes" | "rules" }) {
  const viewer = useViewer();
  const settings = scope === "admin" ? canWrite("payments.write", viewer) : canChannelAction("paymentSettings", viewer);
  const items = [{ key: "orders", label: "支付订单", href: `/${scope}/payments` }, { key: "lanes", label: "支付通道", href: `/${scope}/payments/lanes` }, ...(settings ? [{ key: "rules", label: "收银台规则", href: `/${scope}/payments/rules` }] : [])];
  return <nav aria-label="支付管理" className="flex gap-2 border-b border-hairline pb-3">{items.map(item => <Link key={item.key} href={item.href} aria-current={active === item.key ? "page" : undefined} className={`rounded-control px-3 py-2 text-sm ${active === item.key ? "bg-brand-soft text-brand-emphasis" : "text-ink-secondary"}`}>{item.label}</Link>)}</nav>;
}
