"use client";

import { useState } from "react";
import Link from "next/link";
import { Badge } from "@/components/ui/badge";
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
    <div className="flex flex-col gap-14">
      <section id="models">
        <div className="mb-6 flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Model catalog</p>
            <h2 className="mt-1 text-3xl font-semibold tracking-tight">可用模型</h2>
            <p className="mt-2 text-sm text-slate-400">按当前域名的品牌和渠道白名单展示，不含 Provider 路由。</p>
          </div>
          <Badge>{models.length} 个模型</Badge>
        </div>
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {models.map((model) => (
            <Card key={model.id} className="group p-5 transition hover:-translate-y-0.5 hover:border-white/20">
              <div className="mb-3 flex items-center justify-between gap-2">
                <Badge tone="brand">{model.vendor || "model"}</Badge>
                <span className="th-code text-[11px] text-slate-500">{model.id}</span>
              </div>
              <CardTitle className="mb-1 text-lg font-medium">{model.display_name || model.id}</CardTitle>
              <p className="text-sm text-slate-400">OpenAI / Anthropic 兼容入口可直接调用。</p>
            </Card>
          ))}
        </div>
      </section>
      <section id="plans">
        <div className="mb-6">
          <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Pricing</p>
          <h2 className="mt-1 text-3xl font-semibold tracking-tight">套餐与订阅</h2>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          {plans.map((plan) => (
            <Card key={plan.id} className="p-6">
              <CardTitle className="text-lg font-medium">{plan.name}</CardTitle>
              <p className="mt-2 text-3xl font-semibold">
                {((plan.price_minor ?? 0) / 1_000_000).toString()}
                <span className="ml-1 text-sm font-normal text-slate-400">USD / 月</span>
              </p>
              <Button className="mt-5" onClick={() => subscribe(plan.id || "")}>
                订阅
              </Button>
            </Card>
          ))}
        </div>
      </section>
      <Card id="topup" className="p-6 md:p-8">
        <CardTitle className="mb-2 text-2xl font-semibold">充值</CardTitle>
        <p className="mb-5 text-sm text-slate-400">兑换码或创建 1 USD 的 Stripe 沙箱充值单。未登录会引导去登录，回来后继续购买。</p>
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
        <p className="mt-4 text-sm text-slate-300">{message}</p>
      </Card>
    </div>
  );
}
