"use client";
import { PaymentsNav } from "@/app/admin/payments/payments-nav";
export function ChannelPaymentsNav({ active = "orders" }: { active?: "orders" | "lanes" | "rules" }) { return <PaymentsNav scope="channel" active={active} />; }
