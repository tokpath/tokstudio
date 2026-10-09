"use client";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { useEffect, useState, useTransition } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/client";
import { type Customer, type CustomerScope } from "@/lib/customer";
import { appendReturnContext } from "@/lib/return-context";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
type Page = {
    items: Customer[];
    total: number;
    limit: number;
    next_cursor: string;
};
export function CustomerList({ surface, channelID }: {
    surface: "admin" | "channel";
    channelID?: string;
}) {
    const t = useTranslations("customerExperience");
    const viewer = useViewer();
    const brand = useBrand();
    const pathname = usePathname();
    const search = useSearchParams();
    const router = useRouter();
    const [navigating,startNavigation]=useTransition();
    const scope = `${viewer.userId || ""}:${brand?.id || ""}`;
    const otherScope = search.has("viewer_scope") && search.get("viewer_scope") !== scope;
    const q = otherScope ? "" : search.get("q") || "";
    const status = otherScope ? "" : search.get("status") || "";
    const channel = channelID || (otherScope ? "" : search.get("channel_id") || "");
    const cursor = otherScope ? "" : search.get("cursor") || "";
    const [draft, setDraft] = useState(q);
    useEffect(() => setDraft(q), [q]);
    useEffect(() => { if (!viewer.loading && otherScope)
        router.replace(pathname); }, [viewer.loading, otherScope, pathname, router]);
    const filters = new URLSearchParams({ limit: "25" });
    if (q)
        filters.set("q", q);
    if (status)
        filters.set("status", status);
    if (channel)
        filters.set("channel_id", channel);
    if (cursor)
        filters.set("cursor", cursor);
    const key = filters.toString();
    const query = useQuery({ queryKey: ["customers", surface, scope, viewer.roles.join(","), key], enabled: !viewer.loading && !otherScope, queryFn: () => apiClient<Page>("GET", `/${surface}/customers?${key}`) });
    const scopes = useQuery({ queryKey: ["customer-scopes", surface, scope], enabled: !viewer.loading, queryFn: () => apiClient<{
            items: CustomerScope[];
        }>("GET", `/${surface}/customer-scopes`) });
    const returnParams = new URLSearchParams(search);
    returnParams.set("viewer_scope", scope);
    if (channel)
        returnParams.set("channel_id", channel);
    const returnTo = `${pathname}?${returnParams}`;
    function navigate(values: Record<string, string>) { const params = new URLSearchParams(search); params.set("viewer_scope", scope); params.delete("cursor"); for (const [key, value] of Object.entries(values)) {
        if (value)
            params.set(key, value);
        else
            params.delete(key);
    } startNavigation(()=>router.push(`${pathname}?${params}`)); }
    const selectClass = "h-10 rounded-control border border-hairline bg-canvas px-3 text-sm";
    return <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-4">
  <div className="flex items-center justify-between gap-3"><h2 className="text-lg font-semibold">{t("customers")}</h2><Button size="sm" variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>{t("refresh")}</Button></div>
  <form method="get" onSubmit={e => { e.preventDefault(); navigate({ q: draft.trim() }); }} className="flex flex-wrap gap-2">
   <Input className="max-w-sm" aria-label={t("searchCustomers")} placeholder={t("emailNameChannelOrInvitationCode")} maxLength={200} value={draft} onChange={e => setDraft(e.target.value)}/><Button type="submit" disabled={navigating}>{t("search")}</Button>
   <select className={selectClass} aria-label={t("customerStatus")} disabled={navigating} value={status} onChange={e => navigate({ status: e.target.value })}><option value="">{t("allStatuses")}</option><option value="active">{t("active")}</option><option value="banned">{t("banned")}</option></select>
   {!channelID && <select className={selectClass} aria-label={t("customerChannel")} disabled={navigating||scopes.isPending||scopes.isError} value={channel} onChange={e => navigate({ channel_id: e.target.value })}><option value="">{t("allAccessibleChannels")}</option>{scopes.data?.items.map(item => <option key={item.id} value={item.id}>{item.brand_name} · {item.code}</option>)}</select>}
   {(q || status || cursor) && <Button type="button" variant="outline" onClick={() => navigate({ q: "", status: "", cursor: "" })}>{t("clearSearch")}</Button>}
  </form>
  {scopes.isError && <p role="alert">{t("channelFilterUnavailable")}</p>}
  {query.isPending && <p role="status">{t("loadingCustomers")}</p>}
  {query.isError && <div role="alert"><p>{query.error.message}</p><Button variant="outline" disabled={query.isFetching || navigating} onClick={() => cursor ? navigate({ cursor: "" }) : void query.refetch()}>{t("retryFromFirstPage")}</Button></div>}
  {query.data && <><p className="text-sm text-ink-secondary">{t("customersMatchedOnThisPage", { value0: query.data.total, value1: query.data.items.length })}</p><div className="overflow-x-auto"><table className="min-w-full text-left text-sm"><thead><tr>{[t("customers"), t("brandChannel"), t("status"), t("registered"), ""].map((title, i) => <th className="border-b border-hairline px-3 py-2" key={i}>{title}</th>)}</tr></thead><tbody>{query.data.items.map(item => <tr key={item.id} className="border-b border-hairline"><td className="px-3 py-3"><Link href={appendReturnContext(`/${surface}/users/${encodeURIComponent(item.id)}`, returnTo)} className="text-brand-emphasis underline">{item.display_name || item.email}</Link>{item.display_name && <p className="text-ink-mute break-all">{item.email}</p>}</td><td className="px-3 py-3">{item.brand_name || "—"}<p className="text-ink-mute">{item.channel_code || t("directCustomer")}</p></td><td className="px-3 py-3">{item.status === "active" ? t("active") : item.status === "banned" ? t("banned") : item.status}</td><td className="px-3 py-3 whitespace-nowrap">{new Date(item.created_at).toLocaleDateString()}</td><td className="px-3 py-3"><Link href={appendReturnContext(`/${surface}/users/${encodeURIComponent(item.id)}`, returnTo)} className="text-brand-emphasis underline">{t("viewDetails")}</Link></td></tr>)}</tbody></table></div>{!query.data.items.length && <p>{t("noMatchingCustomers")}</p>}
  <div className="flex gap-3"><Button variant="outline" disabled={!cursor||navigating} onClick={() => navigate({ cursor: "" })}>{t("firstPage")}</Button><Button variant="outline" disabled={!query.data.next_cursor || query.isFetching || navigating} onClick={() => navigate({ cursor: query.data!.next_cursor })}>{t("nextPage")}</Button></div></>}
 </section>;
}
