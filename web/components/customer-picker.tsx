"use client";
import { useTranslations } from "next-intl";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { apiClient } from "@/lib/client";
import type { Customer } from "@/lib/customer";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
export function CustomerPicker({ surface, channelID, value, onChange }: {
    surface: "admin" | "channel";
    channelID: string;
    value: Customer | null;
    onChange: (value: Customer | null) => void;
}) {
    const t = useTranslations("customerExperience");
    const viewer = useViewer();
    const brand = useBrand();
    const [text, setText] = useState("");
    const [q, setQ] = useState("");
    const [cursor, setCursor] = useState("");
    const scope = `${viewer.userId}:${brand?.id}:${channelID}`;
    useEffect(() => { setText(""); setQ(""); setCursor(""); onChange(null); }, [scope]); // eslint-disable-line react-hooks/exhaustive-deps
    const query = useQuery({ queryKey: ["customer-picker", surface, scope, viewer.roles.join(","), q, cursor], enabled: !!channelID && !viewer.loading, queryFn: () => apiClient<{
            items: Customer[];
            next_cursor: string;
        }>("GET", `/${surface}/customers?channel_id=${encodeURIComponent(channelID)}&status=active&limit=25&q=${encodeURIComponent(q)}&cursor=${encodeURIComponent(cursor)}`) });
    const eligible = query.data?.items.filter(item => !item.roles?.some(role => role !== "end_user")) || [];
    return <div className="space-y-3"><div className="flex gap-2"><Input aria-label={t("findRegisteredCustomer")} placeholder={t("registeredCustomerSEmailOrName")} value={text} onChange={e => setText(e.target.value)}/><Button type="button" variant="outline" disabled={!channelID} onClick={() => { setQ(text.trim()); setCursor(""); onChange(null); }}>{t("find")}</Button></div>
 {query.isError && <p role="alert">{t("couldNotLoadCustomersRetry")}</p>}{channelID && query.isPending && <p role="status">{t("loadingCustomers")}</p>}
 <select aria-label={t("selectRegisteredCustomer")} className="h-10 w-full rounded-control border border-hairline bg-canvas px-3" value={value?.id || ""} disabled={!channelID || query.isError || query.isPending} onChange={e => onChange(eligible.find(item => item.id === e.target.value) || null)}><option value="">{t("selectARegisteredCustomerInThisChannel")}</option>{eligible.map(item => <option key={item.id} value={item.id}>{item.display_name || item.email} · {item.email} · {item.channel_code}</option>)}</select>
 {!query.isPending && !query.isError && channelID && !eligible.length && <p className="text-sm text-ink-mute">{t("noEligibleCustomersRegisterThroughThisChannelS")}</p>}
 {query.data?.next_cursor && <Button type="button" size="sm" variant="outline" onClick={() => { setCursor(query.data!.next_cursor); onChange(null); }}>{t("moreCustomers")}</Button>}
 {value && <p className="text-sm text-ink-secondary">{value.email} · {value.brand_name} / {value.channel_code} {t("registeredApiCustomer")}</p>}
 </div>;
}
