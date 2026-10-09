"use client";
import { useTranslations } from "next-intl";
import { Suspense, useEffect, useState, useTransition } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { canChannelAction } from "@/lib/rbac";
import { appendReturnContext } from "@/lib/return-context";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "@/components/confirm-button";
import { OEMPage } from "../management-panels";
type Channel = {
    id: string;
    code: string;
    status: string;
};
function ChannelList() {
    const t = useTranslations("customerExperience");
    const viewer = useViewer();
    const brand = useBrand();
    const search = useSearchParams();
    const pathname = usePathname();
    const router = useRouter();
    const [navigating,startNavigation]=useTransition();
    const scope = `${viewer.userId || ""}:${brand?.id || ""}`;
    const foreign = search.has("viewer_scope") && search.get("viewer_scope") !== scope;
    const q = foreign ? "" : search.get("q") || "";
    const cursor = foreign ? "" : search.get("cursor") || "";
    const [draft, setDraft] = useState(q);
    const [code, setCode] = useState("");
    const [message, setMessage] = useState("");
    useEffect(() => setDraft(q), [q]);
    useEffect(() => { if (!viewer.loading && foreign)
        router.replace(pathname); setCode(""); setMessage(""); }, [scope, foreign, viewer.loading, router, pathname]);
    const query = useQuery({ queryKey: ["subchannels", scope, q, cursor], enabled: !viewer.loading && !foreign, queryFn: () => apiClient<{
            items: Channel[];
            total: number;
            next_cursor: string;
        }>("GET", `/channel/subchannels?q=${encodeURIComponent(q)}&cursor=${encodeURIComponent(cursor)}`) });
    const params = new URLSearchParams(search);
    params.set("viewer_scope", scope);
    const here = `${pathname}?${params}`;
    function navigate(values: Record<string, string>) { const params = new URLSearchParams(search); params.set("viewer_scope", scope); params.delete("cursor"); for (const [key, value] of Object.entries(values)) {
        if (value)
            params.set(key, value);
        else
            params.delete(key);
    } startNavigation(()=>router.push(`${pathname}?${params}`)); }
    async function create() { try {
        const body = await apiClient<{
            item?: Channel;
            error?: {
                message: string;
            };
        }>("POST", "/admin/channels", { headers: confirmHeaders, body: JSON.stringify({ code: code.trim(), type: "B", status: "active" }) });
        if (body.error || !body.item) {
            setMessage(body.error?.message || t("creationDidNotCompleteRetry"));
            return false;
        }
        router.push(appendReturnContext(`/channel/subchannels/${encodeURIComponent(body.item.id)}`, here));
        return true;
    }
    catch {
        setMessage(confirmNetworkUnavailable);
        return false;
    } }
    return <OEMPage page="channels"><div className="flex flex-wrap items-center gap-4 text-sm"><Link href="/channel/models" className="text-brand-emphasis underline">{t("brandModelAccess")}</Link><span className="text-ink-secondary">{t("directChannelsShareThisBrandSPlansAnd")}</span></div>
 {canChannelAction("operations", viewer) && <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-3"><h2 className="font-semibold">{t("createDirectChannel")}</h2><Input className="max-w-lg" aria-label={t("newChannelName")} placeholder={t("channelName")} value={code} maxLength={80} onChange={e => setCode(e.target.value)}/><ConfirmButton disabled={!code.trim()} title={t("confirmDirectChannelCreation")} description={t("channelInheritsThisBrandAndUnifiedPricesContinue", { value0: code.trim() })} onConfirm={create}>{t("createAndContinueSetup")}</ConfirmButton>{message && <p role="status">{message}</p>}</section>}
 <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-4"><div className="flex justify-between gap-3"><h2 className="font-semibold">{t("channelList")}</h2><Button size="sm" variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>{t("refresh")}</Button></div><form method="get" className="flex gap-2" onSubmit={e => { e.preventDefault(); navigate({ q: draft.trim() }); }}><Input aria-label={t("searchChannels")} placeholder={t("channelName")} value={draft} maxLength={200} onChange={e => setDraft(e.target.value)}/><Button type="submit" disabled={navigating}>{t("search")}</Button></form>
 {query.isPending && <p role="status">{t("loadingChannels")}</p>}{query.isError && <div role="alert"><p>{query.error.message}</p><Button variant="outline" onClick={() => navigate({ cursor: "" })}>{t("retryFromFirstPage")}</Button></div>}{query.data && <><p className="text-sm text-ink-secondary">{t("channelsMatchedOnThisPage", { value0: query.data.total, value1: query.data.items.length })}</p><ul className="divide-y divide-hairline">{query.data.items.map(item => <li key={item.id} className="flex justify-between gap-3 py-3"><span>{item.code}<span className="ml-3 text-sm text-ink-secondary">{item.status === "active" ? t("running") : t("disabled")}</span></span><Link className="text-brand-emphasis underline" href={appendReturnContext(`/channel/subchannels/${encodeURIComponent(item.id)}`, here)}>{t("manage")}</Link></li>)}</ul>{!query.data.items.length && <p>{t("noMatchingDirectChannels")}</p>}<div className="flex gap-3"><Button variant="outline" disabled={!cursor||navigating} onClick={() => navigate({ cursor: "" })}>{t("firstPage")}</Button><Button variant="outline" disabled={!query.data.next_cursor || query.isFetching || navigating} onClick={() => navigate({ cursor: query.data!.next_cursor })}>{t("nextPage")}</Button></div></>}
 </section></OEMPage>;
}
export default function Page() { const t = useTranslations("customerExperience"); return <Suspense><ChannelList /></Suspense>; }
