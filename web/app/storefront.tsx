"use client";

import { useState } from "react";
import Link from "next/link";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { loginHref } from "@/lib/login-next";
import { Boxes, CreditCard, Ticket } from "lucide-react";
import { IconStamp } from "@/components/icon-stamp";
import { useTranslations } from "next-intl";

type PublicModel = { id?: string; display_name?: string; vendor?: string };
type PublicPlan = { id?: string; name?: string; price_minor?: number };

export default function PublicStorefront({
  models,
  plans,
}: {
  models: PublicModel[];
  plans: PublicPlan[];
}) {
  const t = useTranslations("storefront");
  const [message, setMessage] = useState("");
  const redeemForm = useForm<{ code: string }>({
    resolver: zodResolver(z.object({ code: z.string().trim().min(1, t("needCode")) })),
    defaultValues: { code: "THE2E" },
  });

  function unauthorizedMessage(status: number, apiMessage?: string, fallback = t("loginFirst")) {
    if (status === 401 || status === 403) {
      return apiMessage || t("loginToBuy");
    }
    return apiMessage || fallback;
  }

  async function redeem(values: { code: string }) {
    const response = await fetch(`${apiBase}/v1/topups/redeem`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code: values.code }),
    });
    const body = await response.json();
    setMessage(response.ok ? t("redeemOk", { amount: body.item?.amount_minor ?? 0 }) : unauthorizedMessage(response.status, body.error?.message, t("loginToTopup")));
  }

  async function topup() {
    const response = await fetch(`${apiBase}/v1/topups`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ amount_minor: 1_000_000, payment_method: "stripe" }),
    });
    const body = await response.json();
    setMessage(response.ok ? t("topupOk", { id: body.item?.id || "" }) : unauthorizedMessage(response.status, body.error?.message, t("loginToTopup")));
  }

  async function subscribe(planId: string) {
    const response = await fetch(`${apiBase}/v1/me/subscriptions`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan_id: planId, adapter: "stripe" }),
    });
    const body = await response.json();
    setMessage(response.ok ? t("ordered", { id: body.checkout?.order?.id || "" }) : unauthorizedMessage(response.status, body.error?.message, t("loginToSubscribe")));
  }

  return (
    <div className="flex flex-col gap-24">
      <section id="models">
        <div className="mb-8 flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="th-eyebrow text-ink-mute">{t("modelsEyebrow")}</p>
            <h2 className="mt-3 text-2xl font-semibold tracking-tight">{t("modelsTitle")}</h2>
            <p className="mt-3 max-w-xl text-base leading-relaxed text-ink-secondary">{t("modelsLead")}</p>
          </div>
          <Badge>{t("modelCount", { count: models.length })}</Badge>
        </div>
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {models.map((model) => (
            <Card key={model.id} className="p-5">
              <div className="mb-3 flex items-center justify-between gap-2">
                <Badge tone="brand">{model.vendor || "model"}</Badge>
                <IconStamp icon={Boxes} size="sm" />
              </div>
              <CardTitle className="mb-1 text-lg font-medium">{model.display_name || model.id}</CardTitle>
              <p className="th-code mb-2 text-[11px] text-ink-mute">{model.id}</p>
              <p className="text-sm text-ink-secondary">{t("compat")}</p>
            </Card>
          ))}
        </div>
      </section>
      <section id="plans">
        <div className="mb-8">
          <p className="th-eyebrow text-ink-mute">{t("plansEyebrow")}</p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">{t("plansTitle")}</h2>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          {plans.map((plan) => (
            <Card key={plan.id} className="p-6">
              <div className="flex items-start justify-between gap-3">
                <CardTitle className="text-lg font-medium">{plan.name}</CardTitle>
                <IconStamp icon={CreditCard} size="sm" />
              </div>
              <p className="mt-2 text-3xl font-semibold">
                {((plan.price_minor ?? 0) / 1_000_000).toString()}
                <span className="ml-1 text-sm font-normal text-ink-secondary">{t("perMonth")}</span>
              </p>
              <Button className="mt-5" onClick={() => subscribe(plan.id || "")}>
                {t("subscribe")}
              </Button>
            </Card>
          ))}
        </div>
      </section>
      <Card id="topup" className="p-8 md:p-10">
        <div className="flex items-start justify-between gap-3">
          <p className="th-eyebrow text-ink-mute">{t("topupEyebrow")}</p>
          <IconStamp icon={Ticket} size="sm" />
        </div>
        <CardTitle className="mb-2 mt-2 text-2xl font-semibold">{t("topupTitle")}</CardTitle>
        <p className="mb-5 text-sm text-ink-secondary">{t("topupLead")}</p>
        <Form {...redeemForm}>
        <form className="flex flex-wrap items-end gap-3" onSubmit={redeemForm.handleSubmit(redeem)}>
          <TextField control={redeemForm.control} name="code" label={t("redeemCode")} showLabel={false} className="max-w-xs" icon={Ticket} />
          <Button type="submit" variant="outline">
            {t("redeem")}
          </Button>
          <Button type="button" onClick={topup}>
            {t("pay")}
          </Button>
          <Button variant="outline" asChild>
            <Link href={loginHref("/")}>{t("goLogin")}</Link>
          </Button>
        </form>
        </Form>
        <p className="mt-4 text-sm text-ink-secondary">{message || t("guestHint")}</p>
      </Card>
    </div>
  );
}
