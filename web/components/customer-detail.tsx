"use client";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { usePathname, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { type Customer, customerReturnHref } from "@/lib/customer";
import { appendReturnContext } from "@/lib/return-context";
import { formatUsdMinor } from "@/lib/money";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmDialog } from "@/components/confirm-button";
import { CreatePartnerDialog } from "@/app/admin/channels/create-dialogs";
type Detail = {
    item: Customer;
    permissions: {
        finance: boolean;
        operations: boolean;
        audit: boolean;
        manage: boolean;
        attribution: boolean;
        record_payment: boolean;
        read_payments: boolean;
        professional_create: boolean;
    };
    errors: Record<string, string>;
    usage?: {
        confirmed_count: number;
        pending_count: number;
        confirmed_minor: number;
        refunded_minor: number;
    };
    credit?: {
        available_minor: number;
        reserved_minor: number;
        gift_minor: number;
    };
    orders?: {
        count: number;
        paid: number;
        pending: number;
        refunded: number;
    };
    activity?: {
        count: number;
        items: {
            request_id: string;
            public_model_id: string;
            status: string;
            started_at: string;
        }[];
    };
    entitlements?: {
        id: string;
        unit_type: string;
        remaining: number;
        expires_at?: string;
        status: string;
    }[];
};
export function CustomerDetail({ surface, id }: {
    surface: "admin" | "channel";
    id: string;
}) {
    const t = useTranslations("customerExperience");
    const viewer = useViewer();
    const brand = useBrand();
    const pathname = usePathname();
    const search = useSearchParams();
    const scope = `${viewer.userId || ""}:${brand?.id || ""}`;
    const operationScope = `${scope}:${id}:${viewer.roles.join(",")}`;
    const scopeRef = useRef(operationScope);
    scopeRef.current = operationScope;
    const query = useQuery({ queryKey: ["customer-detail", surface, scope, viewer.roles.join(","), id], enabled: !viewer.loading, queryFn: () => apiClient<Detail>("GET", `/${surface}/customers/${encodeURIComponent(id)}`) });
    const [action, setAction] = useState<"ban" | "unban" | "attribution" | null>(null);
    const [reason, setReason] = useState("");
    const [promo, setPromo] = useState("");
    const [confirm, setConfirm] = useState(false);
    const [error, setError] = useState("");
    const [message, setMessage] = useState("");
    const item = query.data?.item;
    const permissions = query.data?.permissions;
    const [createProfessional, setCreateProfessional] = useState(false);
    useEffect(() => { setAction(null); setReason(""); setPromo(""); setConfirm(false); setError(""); setMessage(""); setCreateProfessional(false); }, [operationScope]);
    const [promoSearch, setPromoSearch] = useState("");
    const [promoQ, setPromoQ] = useState("");
    const promotions = useQuery({ queryKey: ["customer-promotions", scope, promoQ], enabled: action === "attribution", queryFn: () => apiClient<{
            items: {
                code: string;
                channel_name: string;
                customer_name: string;
            }[];
        }>("GET", `/admin/customer-promotions?q=${encodeURIComponent(promoQ)}`) });
    const back = customerReturnHref(search.get("return_to"), `/${surface}/users`, scope);
    const here = pathname + (search.size ? `?${search}` : "");
    const task = (href: string) => appendReturnContext(href, here);
    async function apply() { if (!action || !item || !reason.trim())
        return false; try {
        const response = await fetch(`${apiBase}/${surface}/users/${encodeURIComponent(id)}/${action}`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ reason: reason.trim(), ...(action === "attribution" ? { promotion_code: promo.trim() } : {}) }) });
        const body = await response.json();
        if (scopeRef.current !== operationScope)
            return false;
        if (!response.ok) {
            setError(body.error?.message || t("actionDidNotComplete"));
            return false;
        }
        setMessage(t("customerStatusSavedCheckTheUpdatedDetails"));
        setAction(null);
        void query.refetch();
        return true;
    }
    catch {
        setError(confirmNetworkUnavailable);
        return false;
    } }
    const actionLabel = action === "ban" ? t("ban") : action === "unban" ? t("unban") : t("changeAttribution");
    return <div className="space-y-6">
  <header className="flex flex-wrap items-start justify-between gap-4"><div><Link href={back} className="text-brand-emphasis underline">{t("backToCustomers")}</Link><h1 className="mt-3 text-2xl font-semibold">{item?.display_name || item?.email || t("customerDetails")}</h1>{item?.display_name && <p className="mt-1 text-ink-secondary">{item.email}</p>}</div><Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>{t("refreshDetails")}</Button></header>
  {query.isPending && <p role="status">{t("loadingCustomers")}</p>}{query.isError && <p role="alert">{query.error.message}</p>}
  {item && permissions && <>
   <section className="rounded-card border border-hairline bg-canvas-raised p-5"><dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">{[[t("brand"), item.brand_name], [t("channel"), item.channel_code || t("directCustomer")], [t("status"), item.status === "active" ? t("active") : item.status === "banned" ? t("banned") : item.status], [t("registered"), new Date(item.created_at).toLocaleString()], [t("customerId"), item.id]].map(([label, value]) => <div key={label}><dt className="text-sm text-ink-mute">{label}</dt><dd className="mt-1 break-all">{value || "—"}</dd></div>)}{permissions.operations && <div><dt className="text-sm text-ink-mute">{t("invitationSource")}</dt><dd className="mt-1">{item.source_code || t("directRegistration")}</dd></div>}</dl></section>
   <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-4"><h2 className="font-semibold">{t("usageAndRequests")}</h2>{query.data?.errors.usage ? <p role="alert">{query.data.errors.usage}</p> : query.data?.usage && <dl className="grid gap-4 sm:grid-cols-4">{[[t("confirmedUsage"), `${formatUsdMinor(query.data.usage.confirmed_minor)} USD`], [t("refundedUsage"), `${formatUsdMinor(query.data.usage.refunded_minor)} USD`], [t("confirmedCharges"), query.data.usage.confirmed_count], [t("awaitingReconciliation"), query.data.usage.pending_count]].map(([label, value]) => <div key={label}><dt className="text-sm text-ink-mute">{label}</dt><dd className="mt-1 font-mono">{value}</dd></div>)}</dl>}
    <Link href={task(`/${surface}/usage?user_id=${encodeURIComponent(id)}`)} className="text-brand-emphasis underline">{t("viewAllUsageCharges")}</Link>
    {query.data?.errors.activity ? <p role="alert">{query.data.errors.activity}</p> : query.data?.activity && <><p className="text-sm text-ink-secondary">{t("requestsLatest", { value0: query.data.activity.count, value1: query.data.activity.items.length })}</p><ul className="divide-y divide-hairline">{query.data.activity.items.map(request => <li key={request.request_id} className="flex flex-wrap justify-between gap-3 py-3 text-sm"><span><span className="font-medium">{request.public_model_id}</span><p className="break-all font-mono text-ink-mute">{request.request_id}</p></span><span>{t.has(`requestStatus_${request.status}`) ? t(`requestStatus_${request.status}`) : t("requestStatus_unknown")}<p className="text-ink-mute">{new Date(request.started_at).toLocaleString()}</p></span><div className="flex gap-3"><Link href={task(`/${surface}/usage/requests/${encodeURIComponent(request.request_id)}`)} className="text-brand-emphasis underline">{t("viewRequest")}</Link></div></li>)}</ul></>}
   </section>
   {permissions.finance && <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-4"><h2 className="font-semibold">{t("creditAndOrders")}</h2>{query.data?.errors.credit ? <p role="alert">{query.data.errors.credit}</p> : query.data?.credit && <dl className="grid gap-4 sm:grid-cols-3">{[[t("available"), query.data.credit.available_minor], [t("reserved"), query.data.credit.reserved_minor], [t("giftCreditIncluded"), query.data.credit.gift_minor]].map(([label, value]) => <div key={label}><dt className="text-sm text-ink-mute">{label}</dt><dd className="mt-1 font-mono">{formatUsdMinor(Number(value))} USD</dd></div>)}</dl>}
    {query.data?.errors.orders ? <p role="alert">{query.data.errors.orders}</p> : query.data?.orders && <p>{t("ordersPaidPendingRefunded", { value0: query.data.orders.count, value1: query.data.orders.paid, value2: query.data.orders.pending, value3: query.data.orders.refunded })}</p>}
    <div className="flex flex-wrap gap-4">{permissions.read_payments && <Link className="text-brand-emphasis underline" href={task(`/${surface}/payments?q=${encodeURIComponent(id)}${surface === "channel" ? `&channel_id=${encodeURIComponent(item.channel_org_id)}` : ""}`)}>{t("viewPaymentOrders")}</Link>}{permissions.record_payment && <Link className="text-brand-emphasis underline" href={task(`/${surface}/payments?customer_id=${encodeURIComponent(id)}`)}>{t("recordOfflinePayment")}</Link>}</div>
    {query.data?.errors.entitlements ? <p role="alert">{query.data.errors.entitlements}</p> : <ul className="divide-y divide-hairline">{query.data?.entitlements?.map(ent => <li key={ent.id} className="flex flex-wrap justify-between gap-3 py-2 text-sm"><span>{ent.unit_type === "usd_credit" ? `${formatUsdMinor(ent.remaining)} USD` : `${ent.remaining} ${ent.unit_type}`} · {ent.status}</span><span>{ent.expires_at ? t("expires", { value0: new Date(ent.expires_at).toLocaleString() }) : t("noExpiration")}</span></li>)}</ul>}
   </section>}
   <div className="flex flex-wrap gap-4">{permissions.operations && item.professional_role_id && <Link href={task(`/${surface}/users/${encodeURIComponent(id)}/promotion?role_id=${encodeURIComponent(item.professional_role_id)}`)} className="text-brand-emphasis underline">{t("viewProfessionalReferralRelationship")}</Link>}{permissions.professional_create && !item.professional_role_id && <Button variant="outline" onClick={() => setCreateProfessional(true)}>{t("setUpProfessionalReferralRelationship")}</Button>}{permissions.audit && <Link href={task(`/${surface}/audit?q=${encodeURIComponent(id)}`)} className="text-brand-emphasis underline">{t("viewCustomerAuditRecords")}</Link>}</div>
   {(permissions.manage || permissions.attribution) && <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-3"><h2 className="font-semibold">{t("customerManagement")}</h2><div className="flex gap-3">{permissions.manage && <Button variant="outline" onClick={() => { setAction(item.status === "banned" ? "unban" : "ban"); setReason(""); setError(""); }}>{item.status === "banned" ? t("unbanCustomer") : t("banCustomer")}</Button>}{permissions.attribution && <Button variant="outline" onClick={() => { setAction("attribution"); setReason(""); setPromo(""); setPromoSearch(""); setPromoQ(""); setError(""); }}>{t("changeInvitationAttribution")}</Button>}</div>{action && <div className="space-y-3"><Input aria-label={t("reason")} placeholder={t("reasonForThisAction")} value={reason} onChange={e => setReason(e.target.value)}/>{action === "attribution" && <div className="space-y-2"><div className="flex gap-2"><Input aria-label={t("searchAttributionTarget")} placeholder={t("channelCustomerEmailOrInvitationCode")} value={promoSearch} onChange={e => setPromoSearch(e.target.value)}/><Button variant="outline" onClick={() => { setPromoQ(promoSearch.trim()); setPromo(""); }}>{t("searchAttribution")}</Button></div><select aria-label={t("invitationAttributionTarget")} className="h-10 w-full rounded-control border border-hairline bg-canvas px-3" disabled={promotions.isPending || promotions.isError} value={promo} onChange={e => setPromo(e.target.value)}><option value="">{t("selectTheVerifiedChannelReferrer")}</option>{promotions.data?.items.map(choice => <option key={choice.code} value={choice.code}>{choice.channel_name} · {choice.customer_name || t("directChannelInvitation")} · {choice.code}</option>)}</select>{promotions.isError && <p role="alert">{t("couldNotLoadInvitationAttributionRetry")}</p>}</div>}<Button disabled={!reason.trim() || (action === "attribution" && !promo.trim())} onClick={() => setConfirm(true)}>{t("reviewAndContinue")}</Button></div>}{message && <p role="status">{message}</p>}</section>}
  </>}
  <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t("confirmCustomerAction", { value0: actionLabel })} description={t("reason2", { value0: item?.email || "", value1: reason.trim(), value2: action === "ban" ? t("disablesLoginAndExistingApiKeysAndPauses") : action === "attribution" ? t("futureAttributionChangesToHistoricalChargesRemainUnchanged", { value0: promo.trim() }) : t("restoresLoginAndNormalCommissionProcessing") })} error={error} onConfirm={apply}/>
  {item && permissions?.professional_create && <CreatePartnerDialog open={createProfessional} onOpenChange={setCreateProfessional} defaultType="agent" defaultCustomer={item}/>}
 </div>;
}
