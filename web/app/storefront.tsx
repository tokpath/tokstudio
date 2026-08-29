"use client";

import { useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { loginHref } from "@/lib/login-next";

type PublicModel = { id?: string; display_name?: string; vendor?: string };
type PublicPlan = { id?: string; name?: string; price_minor?: number };

function unauthorizedMessage(status: number, apiMessage?: string, fallback = "请先登录") {
  if (status === 401 || status === 403) {
    return apiMessage || "未登录，请先点「去登录」再回来购买";
  }
  return apiMessage || fallback;
}

export default function PublicStorefront({
  models,
  plans,
}: {
  models: PublicModel[];
  plans: PublicPlan[];
}) {
  const [code, setCode] = useState("THE2E");
  const [message, setMessage] = useState("未登录时充值和订阅会提示先登录。金额单位是 micro-USD。");

  async function redeem() {
    const response = await fetch(`${apiBase}/v1/topups/redeem`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });
    const body = await response.json();
    setMessage(response.ok ? `兑换成功 ${body.item?.amount_minor ?? 0} micro-USD` : unauthorizedMessage(response.status, body.error?.message, "请先登录再充值"));
  }

  async function topup() {
    const response = await fetch(`${apiBase}/v1/topups`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ amount_minor: 1_000_000, payment_method: "stripe" }),
    });
    const body = await response.json();
    setMessage(response.ok ? `已创建充值单 ${body.item?.id}` : unauthorizedMessage(response.status, body.error?.message, "请先登录再充值"));
  }

  async function subscribe(planId: string) {
    const response = await fetch(`${apiBase}/v1/me/subscriptions`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan_id: planId, adapter: "stripe" }),
    });
    const body = await response.json();
    setMessage(response.ok ? `已下单 ${body.checkout?.order?.id}` : unauthorizedMessage(response.status, body.error?.message, "请先登录再订阅"));
  }

  return (
    <div className="flex flex-col gap-6">
      <section>
        <h2 className="mb-3 text-2xl font-medium">可用模型</h2>
        <p className="mb-4 text-sm text-slate-400">按当前域名的品牌和渠道白名单展示，不含 Provider 路由。</p>
        <div className="grid gap-4 md:grid-cols-2">
          {models.map((model) => (
            <Card key={model.id} className="rounded-2xl border border-slate-800 bg-slate-900/70 p-5">
              <CardTitle className="text-lg font-medium">{model.display_name || model.id}</CardTitle>
              <p className="mt-2 text-sm text-slate-300">
                {model.vendor} · {model.id}
              </p>
            </Card>
          ))}
        </div>
      </section>
      <section>
        <h2 className="mb-3 text-2xl font-medium">套餐与订阅</h2>
        <div className="grid gap-4 md:grid-cols-2">
          {plans.map((plan) => (
            <Card key={plan.id} className="rounded-2xl border border-slate-800 bg-slate-900/70 p-5">
              <CardTitle className="text-lg font-medium">{plan.name}</CardTitle>
              <p className="mt-2 text-slate-300">{((plan.price_minor ?? 0) / 1_000_000).toString()} USD / 月</p>
              <Button className="mt-4" onClick={() => subscribe(plan.id || "")}>
                订阅
              </Button>
            </Card>
          ))}
        </div>
      </section>
      <Card className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <CardTitle className="mb-3 text-xl font-medium">充值</CardTitle>
        <p className="mb-4 text-sm text-slate-400">兑换码或创建 1 USD 的 Stripe 沙箱充值单。未登录会引导去登录，回来后继续购买。</p>
        <div className="flex flex-wrap gap-3">
          <Input value={code} onChange={(e) => setCode(e.target.value)} className="max-w-xs" />
          <Button variant="outline" onClick={redeem}>
            兑换码充值
          </Button>
          <Button onClick={topup}>创建支付充值</Button>
          <Button variant="outline" asChild>
            <Link href={loginHref("/")}>去登录</Link>
          </Button>
        </div>
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </Card>
    </div>
  );
}
