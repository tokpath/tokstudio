"use client";
import { useTranslations } from "next-intl";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { apiClient } from "@/lib/client";
import { HttpResponseError } from "@/lib/http-response";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { customerReturnHref, type Customer } from "@/lib/customer";
import { appendReturnContext } from "@/lib/return-context";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
type Item = {
    role: {
        id: string;
        channel_org_id: string;
        type: string;
        parent_id?: string;
        status: string;
    };
    members: Customer[];
    codes: {
        code: string;
        share_url: string;
    }[];
    invited_count: number;
    parent_name: string;
    parent_user_id: string;
};
export function ProfessionalCustomerDetail({ id, surface = "admin" }: {
    id: string;
    surface?: "admin" | "channel";
}) {
    const t = useTranslations("customerExperience");
    const labels: Record<string, string> = { agent: t("agent"), kol_l1: t("directReferralCustomer"), kol_l2: t("downstreamReferralCustomer"), promoter: t("referringCustomer") };
    const pathname = usePathname();
    const search = useSearchParams();
    const viewer = useViewer();
    const brand = useBrand();
    const scope = `${viewer.userId || ""}:${brand?.id || ""}`;
    const [message, setMessage] = useState("");
    const ref = useRef(`${scope}:${id}`);
    ref.current = `${scope}:${id}`;
    useEffect(() => setMessage(""), [scope, id]);
    const query = useQuery({ queryKey: ["professional-detail", scope, viewer.roles.join(","), id], enabled: !viewer.loading, queryFn: () => apiClient<{
            item: Item;
            manage: boolean;
        }>("GET", `/admin/professional-customers/${encodeURIComponent(id)}`) });
    const item = query.data?.item;
    const back = customerReturnHref(search.get("return_to"), `/${surface}/users`, scope);
    const here = pathname + (search.size ? `?${search}` : "");
    const promotionHref = (roleID: string, customerID: string) => `/${surface}/users/${encodeURIComponent(customerID)}/promotion?role_id=${encodeURIComponent(roleID)}`;
    async function change() { if (!item)
        return false; try {
        const body = await apiClient<{
            error?: {
                message: string;
            };
        }>("PATCH", `/admin/acquisition-roles/${encodeURIComponent(id)}`, { headers: confirmHeaders, body: JSON.stringify({ status: item.role.status === "active" ? "disabled" : "active" }) });
        if (ref.current !== `${scope}:${id}`)
            return false;
        if (body.error) {
            setMessage(body.error.message);
            return false;
        }
        const result = await query.refetch();
        setMessage(result.isError ? t("statusSavedButDetailsCouldNotRefreshRetry") : t("referralStatusSaved"));
        return true;
    }
    catch (error) {
        setMessage(error instanceof HttpResponseError && error.status >= 400 && error.status < 500 ? error.message : confirmNetworkUnavailable);
        return false;
    } }
    return <div className="space-y-6"><header className="flex flex-wrap justify-between gap-3"><div><Link className="text-brand-emphasis underline" href={back}>{t("backToList")}</Link><h1 className="mt-3 text-2xl font-semibold">{item?.members[0]?.display_name || item?.members[0]?.email || t("professionalReferralCustomer")}</h1><p className="mt-1 text-ink-secondary">{labels[item?.role.type || ""] || t("referralRelationship")}</p></div>{item && query.data?.manage && <><ConfirmButton disabled={query.isFetching || query.isError} title={t("confirmReferralAction", { value0: item.role.status === "active" ? t("disable") : t("enable") })} description={t("disablingPreventsNewRegistrationsWithThisRelationshipS")} onConfirm={change}>{item.role.status === "active" ? t("disableReferralRelationship") : t("enableReferralRelationship")}</ConfirmButton></>}</header>
 {query.isPending && <p role="status">{t("loadingReferralCustomers")}</p>}{query.isError && <p role="alert">{query.error.message}</p>}{message && <p role="status">{message}</p>}
 {item && <><section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-4"><dl className="grid gap-4 sm:grid-cols-3"><div><dt className="text-sm text-ink-mute">{t("status")}</dt><dd>{item.role.status === "active" ? t("active") : t("disabled")}</dd></div><div><dt className="text-sm text-ink-mute">{t("directInvitations")}</dt><dd>{item.invited_count}</dd></div><div><dt className="text-sm text-ink-mute">{t("parentReferralCustomer")}</dt><dd>{item.role.parent_id ? <Link href={appendReturnContext(promotionHref(item.role.parent_id, item.parent_user_id || "relationships"), here)} className="text-brand-emphasis underline">{item.parent_name || t("unboundCustomer")}</Link> : t("none")}</dd></div></dl>
 {item.members.length ? <ul className="space-y-3">{item.members.map(member => <li key={member.id}><Link href={appendReturnContext(`/${surface}/users/${encodeURIComponent(member.id)}`, here)} className="text-brand-emphasis underline">{member.display_name || member.email}</Link><p className="text-sm text-ink-secondary">{member.email} · {member.brand_name} / {member.channel_code}</p></li>)}</ul> : <p role="status">{t("thisHistoricalRelationshipHasNoLinkedApiCustomer")}</p>}
 </section><section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-3"><h2 className="font-semibold">{t("invitationLinks")}</h2>{item.codes.length ? item.codes.map(code => <div key={code.code}><p className="font-mono text-sm">{code.code}</p><a className="break-all text-brand-emphasis underline" href={code.share_url} target="_blank" rel="noreferrer">{code.share_url}</a></div>) : <p>{t("noValidInvitationCodeCheckTheReferralRelationship")}</p>}</section></>}
 </div>;
}
