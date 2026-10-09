"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { EntitlementsPanel } from "./entitlements-panel";
import { CheckoutPay } from "@/components/checkout-pay";
import { ListResourceView } from "@/components/console/list-resource-view";
import { useListResource } from "@/hooks/use-list-resource";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";
import type { CheckoutOrder, CheckoutPayload } from "@/lib/checkout";
import { walletReturnPath, purchaseIsResolved } from "@/lib/wallet-context";
import { notifyWalletChanged } from "@/lib/wallet-events";
import { planPeriod, planPrice, type PublicPlan } from "@/lib/public-plans";
import { formatUsdMinor } from "@/lib/money";
import { newPurchaseOperation, restorePurchaseOperation, type PurchaseOperation } from "@/lib/purchase-operation";

type Method = { adapter: string; display_name?: string; name?: string; sandbox?: boolean; auto_renew_supported?: boolean };
type Bundle = {plans: PublicPlan[];methods: Method[];methodError: boolean};

export default function PlansPanel() {
  const t = useTranslations("user");
  const a = useTranslations("publicExperience");
  const te = useTranslations("entitlements");
  const purchase = useTranslations("purchaseExperience");
  const search = useSearchParams();
  const viewer = useViewer();
  const brand = useBrand();
  const selected = search.get("plan") || "";
  const next = walletReturnPath(search.get("next"));
  const [adapter, setAdapter] = useState("");
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [message, setMessage] = useState("");
  const [checkout, setCheckout] = useState<CheckoutPayload | null>(null);
  const [pending, setPending] = useState<PurchaseOperation | null>(null);
  const [paid, setPaid] = useState(false);
  const operation = useRef<PurchaseOperation | null>(null);
  const storageKey = `tokenhub_plan_purchase:${viewer.userId || ""}:${brand?.id || ""}`;
  const scopeRef = useRef(storageKey);
  scopeRef.current = storageKey;
  const bundle = useListResource<Bundle>({
    enabled: !viewer.loading,
    queryKey: `${viewer.userId || ""}|${brand?.id || ""}`,
    load: async () => {
      const [planRes,payRes] = await Promise.all([fetch(`${apiBase}/v1/me/plans`,{credentials:"include"}),fetch(`${apiBase}/v1/payments/checkout`,{credentials:"include"}).catch(()=>null)]);
      const planBody = await planRes.json();
      if (!planRes.ok) return {ok:false,status:planRes.status,items:[],message:planBody.error?.message,code:planBody.error?.code};
      const payBody = payRes ? await payRes.json().catch(()=>({})) : {};
      return {ok:true,status:planRes.status,items:[{plans:planBody.items || [],methods:payRes?.ok ? payBody.item?.methods || [] : [],methodError:!payRes?.ok}]};
    },
  });
  const item = bundle.snapshot.items[0];
  const method = item?.methods.find(value => value.adapter === adapter);
  useEffect(() => {
    if (item && !item.methods.some(value=>value.adapter === adapter)) setAdapter(item.methods[0]?.adapter || "");
  }, [item,adapter]);
  useEffect(() => {
    busyRef.current=false;setBusy(false);
    operation.current = null; setPending(null); setCheckout(null); setPaid(false);setMessage("");
    try { const saved = restorePurchaseOperation(sessionStorage.getItem(storageKey)); operation.current = saved; setPending(saved); } catch { /* private mode */ }
  }, [storageKey]);
  function clearOperation() {
    operation.current=null; setPending(null);
    try {sessionStorage.removeItem(storageKey);} catch { /* private mode */ }
  }
  async function checkPurchase(op: PurchaseOperation) {
    try {
      const response = await fetch(`${apiBase}/v1/me/subscription-purchases/${encodeURIComponent(op.id)}`,{credentials:"include"});
      const body = await response.json();
      if (scopeRef.current !== storageKey) return;
      if(response.ok && body.item?.id){
        setCheckout(previous=>({...previous,order:body.item,sandbox:previous?.sandbox || false}));
        if(purchaseIsResolved(body.item))clearOperation();
        if(body.item.status==="paid"){setPaid(!!body.item.fulfilled_at);notifyWalletChanged({userId:viewer.userId || "",brandId:brand?.id || ""});void bundle.reload();}
        return;
      }
    } catch { /* Unknown result retains the original operation. */ }
    if(scopeRef.current===storageKey)setPending(op);
  }
  async function subscribe(planId: string) {
    if (busyRef.current) return;
    busyRef.current=true; setBusy(true);setMessage("");
    const op = operation.current || newPurchaseOperation(planId,adapter,false);
    operation.current=op;setPending(op);setPaid(false);
    try {sessionStorage.setItem(storageKey,JSON.stringify(op));} catch { /* private mode */ }
    try {
      const response = await fetch(`${apiBase}/v1/me/subscriptions`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json","Idempotency-Key":op.id},body:JSON.stringify({plan_id:op.planId,adapter:op.adapter,auto_renew:op.autoRenew})});
      const body = await response.json();
      if (scopeRef.current !== storageKey) return;
      if (!response.ok || !body.checkout?.order?.id) { setMessage(body.error?.message || t("subFail")); await checkPurchase(op); return; }
      setCheckout(body.checkout);setMessage(t("ordered",{id:body.checkout.order.id}));
      if(purchaseIsResolved(body.checkout.order))clearOperation();
      if(body.checkout.order.status==="paid"){setPaid(!!body.checkout.order.fulfilled_at);notifyWalletChanged({userId:viewer.userId || "",brandId:brand?.id || ""});void bundle.reload();}
    } catch {if(scopeRef.current === storageKey){setMessage(purchase("unknown"));await checkPurchase(op);}}
    finally {if(scopeRef.current === storageKey){busyRef.current=false;setBusy(false);}}
  }
  function onPaid(fact:CheckoutOrder) {if(scopeRef.current!==storageKey)return;setPaid(!!fact.fulfilled_at);notifyWalletChanged({userId:viewer.userId || "",brandId:brand?.id || ""});void bundle.reload();}
  function resolved(fact:CheckoutOrder){if(scopeRef.current!==storageKey || fact.id!==checkout?.order?.id)return;setCheckout(previous=>previous ? {...previous,order:{...previous.order,...fact}} : previous);if(purchaseIsResolved(fact))clearOperation();}
  return <section className="flex flex-col gap-5">
    {next && <Link className="self-start text-sm text-brand-emphasis underline" href={next}>{a("returnTask")}</Link>}
    <ListResourceView snapshot={bundle.snapshot} emptyTitle={t("plansEmpty")} emptyDetail={t("plansEmptyDetail")} onRetry={()=>void bundle.reload()}>
      {item && <>
        {selected && !item.plans.some(plan=>plan.id===selected) && <p role="alert" className="mb-4 text-danger">{a("planUnavailable")}</p>}
        {pending && (!checkout || (checkout.order?.status==="pending" && !checkout.sandbox && !checkout.qr_code && !checkout.redirect_url && !checkout.client_secret)) && <Card><p role="status">{purchase("resume")}</p><p className="mt-2 text-sm text-ink-secondary">{purchase("original",{plan:pending.planId,method:pending.adapter})}</p><Button className="mt-3" disabled={busy} onClick={()=>void subscribe(pending.planId)}>{purchase("retryOriginal")}</Button></Card>}
        <div className="mb-4 flex flex-wrap gap-2">{item.methods.map(value=><Button key={value.adapter} variant={adapter===value.adapter ? "default":"outline"} disabled={busy || !!pending} onClick={()=>{setAdapter(value.adapter);}}>{value.display_name || value.name || value.adapter}{value.sandbox ? " · SANDBOX":""}</Button>)}</div>
        {item.methodError ? <p role="alert" className="mb-3 text-danger">{a("failedPaymentMethods")}</p> : !item.methods.length && <p role="status" className="mb-3">{a("noPaymentMethod")}</p>}
        <div className="grid gap-4 md:grid-cols-2">{item.plans.map(plan=>{
          return <Card key={plan.id} className={plan.id===selected ? "border-brand":""}><CardTitle>{plan.name}</CardTitle>{plan.id===selected && <p className="mt-2 text-sm text-brand-emphasis">{a("selectedPlan")}</p>}<p className="my-3 font-mono text-xl">{planPrice(plan)}</p><p className="text-sm text-ink-secondary">{a(`period_${planPeriod(plan.billing_period)}`)}</p><ul className="my-3 space-y-2 text-sm">{plan.items?.map((benefit,i)=><li key={i}>{benefit.unit_type === "usd_credit" ? `${formatUsdMinor(benefit.included_amount)} ${a("giftCredit")}` : a("unknownUnit",{amount:benefit.included_amount,unit:te.has(benefit.unit_type)?te(benefit.unit_type):benefit.unit_type})}{benefit.public_model_id && <span className="block break-all text-ink-secondary">{benefit.public_model_id}</span>}{benefit.expires_in_seconds && benefit.expires_in_seconds > 0 ? <span className="block text-ink-secondary">{a("validDays",{days:benefit.expires_in_seconds/86400})}</span> : null}</li>)}</ul>
            <p className="my-3 text-sm text-ink-secondary">{a("renewManual")}</p>
            <Button disabled={busy || !!pending || !method || item.methodError || planPeriod(plan.billing_period)==="unknown"} onClick={()=>void subscribe(plan.id)}>{t("subscribe")}</Button>
          </Card>;
        })}</div>
        {!item.plans.length && <p>{t("plansEmpty")}</p>}
        <Button variant="outline" className="mt-4" disabled={busy} onClick={()=>void bundle.reload()}>{t("refreshPlans")}</Button>
      </>}
    </ListResourceView>
    {checkout && <CheckoutPay checkout={checkout} onPaid={onPaid} onResolved={resolved} />}
    {checkout && pending && <Button className="self-start" variant="outline" disabled={busy} onClick={()=>void checkPurchase(pending)}>{purchase("checkOriginal")}</Button>}
    {paid && <p role="status">{purchase("paid")}</p>}
    {message && <p role="status" className="text-sm text-ink-secondary">{message}</p>}
    <EntitlementsPanel />
  </section>;
}
