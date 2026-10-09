"use client";
import { useTranslations } from "next-intl";
import { useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { CustomerPicker } from "@/components/customer-picker";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useBrand } from "@/components/brand-context";
import { useViewer } from "@/components/rbac/viewer-context";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { appendReturnContext } from "@/lib/return-context";
import type { Customer, CustomerScope } from "@/lib/customer";
const selectClass = "h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm";
type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
};
export function CreateChannelDialog({ open, onOpenChange }: Props) {
    const t = useTranslations("customerExperience");
    const [code, setCode] = useState("");
    const [message, setMessage] = useState("");
    const router = useRouter();
    const client = useQueryClient();
    const pathname = usePathname();
    const search = useSearchParams();
    async function create() { try {
        const body = await apiClient<{
            item?: {
                id: string;
            };
            error?: {
                message: string;
            };
        }>("POST", "/admin/channels", { headers: confirmHeaders, body: JSON.stringify({ code: code.trim(), type: "B", status: "active" }) });
        if (body.error || !body.item?.id) {
            setMessage(body.error?.message || t("creationDidNotCompleteRetry"));
            return false;
        }
        void client.invalidateQueries({ queryKey: ["/admin/channels"] });
        onOpenChange(false);
        router.push(appendReturnContext(`/admin/channels/${encodeURIComponent(body.item.id)}`, pathname + (search.size ? `?${search}` : "")));
        return true;
    }
    catch {
        setMessage(confirmNetworkUnavailable);
        return false;
    } }
    return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><DialogHeader><DialogTitle>{t("createDirectChannel")}</DialogTitle><DialogDescription>{t("inheritsThePlatformBrandPlansAndUnifiedPrices")}</DialogDescription></DialogHeader><label className="space-y-2">{t("channelName")}<Input aria-label={t("channelName")} value={code} onChange={e => setCode(e.target.value)} maxLength={80}/></label><ConfirmButton disabled={!code.trim()} title={t("confirmDirectChannelCreation")} description={t("channelInheritsThePlatformBrandWithoutSeparatePayment", { value0: code.trim() })} onConfirm={create}>{t("createAndContinueSetup")}</ConfirmButton>{message && <p role="status">{message}</p>}</DialogContent></Dialog>;
}
export function CreatePartnerDialog({ open, onOpenChange, defaultType, defaultCustomer }: Props & {
    defaultType: "agent" | "kol_l1" | "kol_l2";
    defaultCustomer?: Customer;
}) {
    const t = useTranslations("customerExperience");
    const viewer = useViewer();
    const brand = useBrand();
    const surface = viewer.roles.includes("platform_admin") ? "admin" : "channel";
    const router = useRouter();
    const client = useQueryClient();
    const pathname = usePathname();
    const search = useSearchParams();
    const [channel, setChannel] = useState("");
    const [customer, setCustomer] = useState<Customer | null>(null);
    const [type, setType] = useState(defaultType);
    const [parent, setParent] = useState("");
    const [parentText, setParentText] = useState("");
    const [parentQ, setParentQ] = useState("");
    const [message, setMessage] = useState("");
    useEffect(() => { if (open) {
        setType(defaultType);
        setParentText("");
        setParentQ("");
        setChannel(defaultCustomer?.channel_org_id || "");
        setCustomer(defaultCustomer || null);
        setParent("");
        setMessage("");
    } }, [open, defaultType, defaultCustomer, viewer.userId, brand?.id]);
    const scopes = useQuery({ queryKey: ["professional-scopes", viewer.userId, brand?.id, surface], enabled: open, queryFn: () => apiClient<{
            items: CustomerScope[];
        }>("GET", `/${surface}/customer-scopes`) });
    const parents = useQuery({ queryKey: ["professional-parents", viewer.userId, brand?.id, channel, type, parentQ], enabled: open && !!channel && type !== "agent", queryFn: () => apiClient<{
            items: {
                id: string;
                name: string;
                type: string;
            }[];
        }>("GET", `/admin/professional-customers?channel_id=${encodeURIComponent(channel)}&type=${type === "kol_l2" ? "kol_l1" : "agent"}&q=${encodeURIComponent(parentQ)}`) });
    async function create() { if (!customer)
        return false; try {
        const body = await apiClient<{
            item?: {
                id: string;
            };
            error?: {
                message: string;
            };
        }>("POST", "/admin/professional-customers", { headers: confirmHeaders, body: JSON.stringify({ user_id: customer.id, type, parent_id: parent }) });
        if (body.error || !body.item?.id) {
            setMessage(body.error?.message || t("setupDidNotCompleteRetry"));
            return false;
        }
        void client.invalidateQueries();
        onOpenChange(false);
        router.push(appendReturnContext(surface === "admin" ? `/admin/partners/${encodeURIComponent(body.item.id)}` : `/channel/users/${encodeURIComponent(customer.id)}/promotion?role_id=${encodeURIComponent(body.item.id)}`, pathname + (search.size ? `?${search}` : "")));
        return true;
    }
    catch {
        setMessage(confirmNetworkUnavailable);
        return false;
    } }
    const selectedParent = parents.data?.items.find(item => item.id === parent);
    return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><DialogHeader><DialogTitle>{t("setUpProfessionalReferralCustomer")}</DialogTitle><DialogDescription>{t("selectAnExistingApiCustomerInThisChannel")}</DialogDescription></DialogHeader>
 <label>{t("channel")}<select aria-label={t("channel")} className={selectClass} value={channel} disabled={!!defaultCustomer || scopes.isPending || scopes.isError} onChange={e => { setChannel(e.target.value); setCustomer(null); setParent(""); setParentQ(""); }}><option value="">{t("selectChannel")}</option>{scopes.data?.items.filter(item => item.can_create_professional).map(item => <option key={item.id} value={item.id}>{item.brand_name} · {item.code}</option>)}</select></label>{scopes.isError && <p role="alert">{t("couldNotLoadChannelsCloseAndRetry")}</p>}
 {defaultCustomer ? <p className="rounded-control border border-hairline p-3">{defaultCustomer.email} · {defaultCustomer.brand_name} / {defaultCustomer.channel_code} {t("registeredApiCustomer")}</p> : <CustomerPicker surface={surface} channelID={channel} value={customer} onChange={setCustomer}/>}
 <label>{t("referralRelationship")}<select aria-label={t("referralRelationship")} className={selectClass} value={type} onChange={e => { setType(e.target.value as typeof type); setParent(""); }}><option value="agent">{t("agent")}</option><option value="kol_l1">{t("directReferralCustomer")}</option><option value="kol_l2">{t("downstreamReferralCustomer")}</option></select></label>
 {type !== "agent" && <div className="space-y-2"><div className="flex gap-2"><Input aria-label={t("findParentReferralCustomer")} placeholder={t("parentCustomerSEmailOrName")} value={parentText} onChange={e => setParentText(e.target.value)}/><Button type="button" variant="outline" onClick={() => { setParentQ(parentText.trim()); setParent(""); }}>{t("findParent")}</Button></div><select aria-label={t("parentReferralCustomer")} className={selectClass} disabled={parents.isPending || parents.isError} value={parent} onChange={e => setParent(e.target.value)}><option value="">{t("selectParentCustomerInTheSameChannel")}</option>{parents.data?.items.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select>{parents.isError && <p role="alert">{t("couldNotLoadParentRelationshipsRetry")}</p>}</div>}
 <ConfirmButton disabled={!customer || (type !== "agent" && !parent)} title={t("confirmProfessionalReferralSetup")} description={t("eligibilityRatesAccountOwnershipAndPasswordsStayUnchanged", { value0: customer?.email || "", value1: customer?.brand_name || "", value2: customer?.channel_code || "", value3: type === "agent" ? t("agent") : t("parent2", { value0: selectedParent?.name || "" }) })} onConfirm={create}>{t("setUpReferralRelationship")}</ConfirmButton>{message && <p role="status">{message}</p>}
 </DialogContent></Dialog>;
}
export function OpenCreateButton({ label, onClick }: {
    label: string;
    onClick: () => void;
}) { const t = useTranslations("customerExperience"); return <Button size="sm" onClick={onClick}>{label}</Button>; }
