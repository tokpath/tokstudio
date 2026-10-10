"use client";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { useEffect, useState, useTransition } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { apiClient } from "@/lib/client";
import { appendReturnContext } from "@/lib/return-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
type Row = {
    id: string;
    name: string;
    email: string;
    user_id: string;
    type: string;
    status: string;
    channel_name: string;
    parent_name: string;
};
export function ProfessionalCustomerList({ channelID, surface = "admin" }: {
    channelID: string;
    surface?: "admin" | "channel";
}) {
    const t = useTranslations("customerExperience");
    const viewer = useViewer();
    const brand = useBrand();
    const search = useSearchParams();
    const router = useRouter();
    const [navigating,startNavigation]=useTransition();
    const pathname = usePathname();
    const scope = `${viewer.userId || ""}:${brand?.id || ""}`;
    const foreign = search.has("viewer_scope") && search.get("viewer_scope") !== scope;
    const q = foreign ? "" : search.get("professional_q") || "";
    const cursor = foreign ? "" : search.get("professional_cursor") || "";
    const [text, setText] = useState(q);
    useEffect(() => setText(q), [q]);
    const query = useQuery({ queryKey: ["professional-customers", scope, viewer.roles.join(","), channelID, q, cursor], enabled: !viewer.loading, queryFn: () => apiClient<{
            items: Row[];
            total: number;
            next_cursor: string;
        }>("GET", `/admin/professional-customers/search?channel_id=${encodeURIComponent(channelID)}&q=${encodeURIComponent(q)}&cursor=${encodeURIComponent(cursor)}`) });
    const params = new URLSearchParams(search);
    params.set("viewer_scope", scope);
    const here = `${pathname}?${params}`;
    function navigate(values: Record<string, string>) { const params = new URLSearchParams(search); params.set("viewer_scope", scope); params.delete("professional_cursor"); for (const [key, value] of Object.entries(values)) {
        if (value)
            params.set(key, value);
        else
            params.delete(key);
    } startNavigation(()=>router.push(`${pathname}?${params}`)); }
    const href = (row: Row) => surface === "admin" ? `/admin/partners/${encodeURIComponent(row.id)}` : `/channel/users/${encodeURIComponent(row.user_id || "current")}/promotion?role_id=${encodeURIComponent(row.id)}`;
    const labels: Record<string, string> = { agent: t("agent"), kol_l1: t("directReferralCustomer"), kol_l2: t("downstreamReferralCustomer") };
    return <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-4"><div className="flex justify-between gap-3"><h2 className="font-semibold">{t("professionalReferralCustomer")}</h2><Button variant="outline" size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}>{t("refresh")}</Button></div><form method="get" className="flex gap-2" onSubmit={e => { e.preventDefault(); navigate({ professional_q: text.trim() }); }}><Input aria-label={t("searchReferralCustomers")} placeholder={t("customerEmailOrName")} value={text} onChange={e => setText(e.target.value)} maxLength={200}/><Button type="submit" disabled={navigating}>{t("search")}</Button></form>
 {query.isPending && <p role="status">{t("loadingReferralCustomers")}</p>}{query.isError && <div role="alert"><p>{query.error.message}</p><Button variant="outline" disabled={query.isFetching || navigating} onClick={() => cursor ? navigate({ professional_cursor: "" }) : void query.refetch()}>{t("retryFromFirstPage")}</Button></div>}{query.data && <><p className="text-sm text-ink-secondary">{t("referralCustomersMatchedOnThisPage", { value0: query.data.total, value1: query.data.items.length })}</p><ul className="divide-y divide-hairline">{query.data.items.map(row => <li key={row.id} className="flex flex-wrap justify-between gap-3 py-3"><div><Link href={appendReturnContext(href(row), here)} className="text-brand-emphasis underline">{row.name || t("unboundCustomer")}</Link><p className="text-sm text-ink-mute">{labels[row.type]} · {row.status === "active" ? t("active") : t("disabled")}{row.parent_name ? t("parent", { value0: row.parent_name }) : ""}</p></div>{row.email && <p className="text-sm text-ink-secondary">{row.email}</p>}</li>)}</ul>{!query.data.items.length && <p>{t("noMatchingReferralCustomers")}</p>}<div className="flex gap-3"><Button variant="outline" disabled={!cursor||navigating} onClick={() => navigate({ professional_cursor: "" })}>{t("firstPage")}</Button><Button variant="outline" disabled={!query.data.next_cursor || query.isFetching || navigating} onClick={() => navigate({ professional_cursor: query.data!.next_cursor })}>{t("nextPage")}</Button></div></>}
 </section>;
}
