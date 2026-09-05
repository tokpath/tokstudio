"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Button } from "@/components/ui/button";
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
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [plans, setPlans] = useState<Plan[]>([]);
  const [ents, setEnts] = useState<Entitlement[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [message, setMessage] = useState(t("plansHint"));

  async function refresh() {
    const [planRes, entRes] = await Promise.all([
      fetch(`${apiBase}/v1/me/plans`, { credentials: "include" }),
      fetch(`${apiBase}/v1/me/entitlements`, { credentials: "include" }),
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
      body: JSON.stringify({ plan_id: planId, adapter: "stripe" }),
    });
    const body = await response.json();
    setMessage(response.ok ? t("ordered", { id: body.checkout?.order?.id }) : body.error?.message || t("subFail"));
  }

  const activeEnts = ents.filter((item) => item.status === "active").length;

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <p className="mb-4 text-sm text-ink-secondary">{t("plansLead")}</p>
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
                {plan.name} · {(plan.price_minor / 1_000_000).toString()} USD
              </span>
              <Button type="button" size="sm" onClick={() => subscribe(plan.id)}>
                {t("subscribe")}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {loaded ? <p className="mt-4 text-sm text-ink-secondary">{t("ents", { n: activeEnts })}</p> : null}
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
