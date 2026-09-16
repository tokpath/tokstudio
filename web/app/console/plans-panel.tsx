"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { CheckoutPay } from "@/components/checkout-pay";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";
import type { CheckoutPayload } from "@/lib/checkout";
import { formatUsdMinor } from "@/lib/money";

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

type Method = {
  adapter: string;
  display_name?: string;
  name?: string;
  sandbox?: boolean;
  auto_renew_supported?: boolean;
  brand_color?: string;
};

export default function PlansPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [plans, setPlans] = useState<Plan[]>([]);
  const [ents, setEnts] = useState<Entitlement[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [message, setMessage] = useState(t("plansHint"));
  const [methods, setMethods] = useState<Method[]>([]);
  const [adapter, setAdapter] = useState("stripe");
  const [checkout, setCheckout] = useState<CheckoutPayload | null>(null);

  async function refresh() {
    const [planRes, entRes, payRes] = await Promise.all([
      fetch(`${apiBase}/v1/me/plans`, { credentials: "include" }),
      fetch(`${apiBase}/v1/me/entitlements`, { credentials: "include" }),
      fetch(`${apiBase}/v1/payments/checkout`, { credentials: "include" }),
    ]);
    const planBody = await planRes.json();
    const entBody = await entRes.json();
    if (!planRes.ok) {
      setLoaded(true);
      setMessage(planBody.error?.message || tc("notLoggedIn"));
      return;
    }
    setPlans(planBody.items || []);
    setEnts(entBody.items || []);
    if (payRes.ok) {
      const payBody = await payRes.json();
      const list: Method[] = payBody.item?.methods || [];
      setMethods(list);
      const preferred = list.find((m) => m.adapter === "stripe") || list[0];
      if (preferred) setAdapter(preferred.adapter);
    }
    setLoaded(true);
    setMessage(t("plansRefreshed"));
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function subscribe(planId: string) {
    const response = await fetch(`${apiBase}/v1/me/subscriptions`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan_id: planId, adapter }),
    });
    const body = await response.json();
    if (response.ok) {
      setCheckout(body.checkout || null);
      setMessage(t("ordered", { id: body.checkout?.order?.id }));
    } else {
      setCheckout(null);
      setMessage(body.error?.message || t("subFail"));
    }
  }

  const activeEnts = ents.filter((item) => item.status === "active").length;

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <p className="mb-4 text-sm text-ink-secondary">{t("plansLead")}</p>
      {methods.length > 0 ? (
        <div className="mb-4 flex flex-wrap gap-2">
          {methods.map((method) => (
            <Button
              key={method.adapter}
              type="button"
              size="sm"
              variant={adapter === method.adapter ? "default" : "outline"}
              onClick={() => setAdapter(method.adapter)}
            >
              {method.display_name || method.name || method.adapter}
              {method.sandbox ? " · SANDBOX" : ""}
            </Button>
          ))}
        </div>
      ) : null}
      {methods.find((m) => m.adapter === adapter)?.auto_renew_supported ? (
        <p className="mb-3 rounded-stamp bg-canvas px-3 py-2 text-sm text-ink-secondary">{t("payAutoRenew")}</p>
      ) : methods.find((m) => m.adapter === adapter) ? (
        <p className="mb-3 rounded-stamp bg-canvas px-3 py-2 text-sm text-ink-secondary">{t("payNoAutoRenew")}</p>
      ) : null}
      <Button type="button" variant="outline" className="mb-4" onClick={() => void refresh()}>
        {t("refreshPlans")}
      </Button>
      {loaded && plans.length === 0 ? (
        <EmptyLedger title={t("plansEmpty")} detail={t("plansEmptyDetail")} />
      ) : (
        <ul className="space-y-3 text-sm text-ink">
          {plans.map((plan) => (
            <li key={plan.id} className="flex items-center justify-between gap-3">
              <span>
                {plan.name} · {formatUsdMinor(plan.price_minor)}
              </span>
              <Button type="button" size="sm" onClick={() => subscribe(plan.id)}>
                {t("subscribe")}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {loaded ? <p className="mt-4 text-sm text-ink-secondary">{t("ents", { n: activeEnts })}</p> : null}
      {checkout ? <CheckoutPay checkout={checkout} onPaid={() => void refresh()} /> : null}
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
