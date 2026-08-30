"use client";

import { useState } from "react";
import { apiBase } from "@/lib/api";

type Plan = {
  id: string;
  name: string;
  price_minor: number;
  status: string;
  items?: { unit_type: string; included_amount: number }[];
};

type Entitlement = {
  id: string;
  source_type: string;
  unit_type: string;
  remaining: number;
  status: string;
};

export default function PlansPanel() {
  const [plans, setPlans] = useState<Plan[]>([]);
  const [ents, setEnts] = useState<Entitlement[]>([]);
  const [message, setMessage] = useState("登录后可查看套餐。沙箱支付走 Stripe webhook。");

  async function refresh() {
    const [planRes, entRes] = await Promise.all([
      fetch(`${apiBase}/v1/me/plans`, { credentials: "include" }),
      fetch(`${apiBase}/v1/me/entitlements`, { credentials: "include" }),
    ]);
    const planBody = await planRes.json();
    const entBody = await entRes.json();
    if (!planRes.ok) {
      setMessage(planBody.error?.message || "未登录");
      return;
    }
    setPlans(planBody.items || []);
    setEnts(entBody.items || []);
    setMessage("套餐与权益已刷新");
  }

  async function subscribe(planId: string) {
    const response = await fetch(`${apiBase}/v1/me/subscriptions`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan_id: planId, adapter: "stripe" }),
    });
    const body = await response.json();
    setMessage(response.ok ? `已下单 ${body.checkout?.order?.id}，请走沙箱 webhook` : body.error?.message || "订阅失败");
  }

  return (
    <section className="rounded-2xl border border-white/10 bg-white/[0.035] p-6 shadow-glow">
      <h2 className="mb-3 text-xl font-medium tracking-tight">套餐与权益</h2>
      <p className="mb-4 text-sm text-slate-400">
        扣减顺序：即将过期的赠送 → 当期套餐 → 现金钱包。金额单位是 micro-USD。
      </p>
      <button className="mb-4 rounded border border-slate-600 px-4 py-2" onClick={refresh}>
        刷新套餐
      </button>
      <ul className="space-y-3 text-sm text-slate-200">
        {plans.map((plan) => (
          <li key={plan.id} className="flex items-center justify-between gap-3">
            <span>
              {plan.name} · {(plan.price_minor / 1_000_000).toString()} USD
            </span>
            <button
              className="rounded px-3 py-1 text-slate-950"
              style={{ background: "var(--brand-primary)" }}
              onClick={() => subscribe(plan.id)}
            >
              订阅
            </button>
          </li>
        ))}
      </ul>
      <p className="mt-4 text-sm text-slate-400">
        有效权益 {ents.filter((item) => item.status === "active").length} 条
      </p>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </section>
  );
}
