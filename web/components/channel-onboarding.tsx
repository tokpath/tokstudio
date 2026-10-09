"use client";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { useState } from "react";
import { useBrand } from "@/components/brand-context";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/client";
import { useViewer } from "@/components/rbac/viewer-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
type Onboarding = {
    item: {
        channel: {
            id: string;
            code: string;
            status: string;
        };
        brand_name: string;
        registration_url: string;
        admin_count: number;
        can_manage_admins:boolean;
        can_manage_models:boolean;
    };
    models_available: boolean;
    model_count?: number;
};
export function ChannelOnboardingPanel({ channelID }: {
    channelID: string;
}) {
    const t = useTranslations("customerExperience");
    const viewer = useViewer();
    const brand = useBrand();
    const [message, setMessage] = useState("");
    const query = useQuery({ queryKey: ["channel-onboarding", viewer.userId, brand?.id, channelID], enabled: !viewer.loading, queryFn: () => apiClient<Onboarding>("GET", `/admin/channels/${encodeURIComponent(channelID)}/onboarding`) });
    const item = query.data?.item;
    return <section className="rounded-card border border-hairline bg-canvas-raised p-5 space-y-4"><div className="flex justify-between gap-3"><h2 className="text-lg font-semibold">{t("channelSetup")}</h2><Button size="sm" variant="outline" onClick={() => void query.refetch()}>{t("checkProgress")}</Button></div>
 {query.isPending && <p role="status">{t("loadingSetupStatus")}</p>}{query.isError && <p role="alert">{t("couldNotLoadRetry")}</p>}
 {item && <><p className="text-sm text-ink-secondary">{item.brand_name}{t("brandPlansAndRetailPricesAreManagedBy")}</p><ol className="space-y-3"><li><span className="font-medium">{t("authorizeAvailableModels")}</span><p className="mt-1 text-sm text-ink-secondary">{!query.data?.models_available ? t("couldNotLoadModelAccessCheckTheAuthorization") : query.data.model_count ? t("modelsAuthorized", { value0: query.data.model_count }) : item.can_manage_models ? t("noModelsAuthorizedSelectAvailableBrandModelsBelow"):t("modelSetupByBrandOwner")}</p></li><li><span className="font-medium">{t("inviteTheIncomingAdministratorToRegister")}</span>{item.registration_url ? <div className="mt-2 flex flex-wrap gap-2"><Input aria-label={t("channelRegistrationLink")} readOnly value={item.registration_url}/><Button size="sm" variant="outline" onClick={() => { void navigator.clipboard.writeText(item.registration_url).then(() => setMessage(t("registrationLinkCopied")), () => setMessage(t("copyFailedCopyTheLinkDirectly"))); }}>{t("copyRegistrationLink")}</Button></div> : <p className="mt-1 text-sm text-ink-secondary">{t("channelIsDisabledOrHasNoValidRegistration")}</p>}</li><li><span className="font-medium">{t("assignAndVerifyTheAdministrator")}</span><p className="mt-1 text-sm text-ink-secondary">{item.admin_count ? t("administratorsAssignedTheIncomingAdministratorCanSignIn", { value0: item.admin_count }) : item.can_manage_admins ? t("afterRegistrationSelectTheCustomerBelowAndGrant"):t("adminSetupByBrandOwner")}</p></li></ol><Link href={`/${viewer.roles.includes("platform_admin") ? "admin" : "channel"}/users?channel_id=${encodeURIComponent(channelID)}`} className="inline-block text-brand-emphasis underline">{t("viewChannelCustomers")}</Link></>}
 {message && <p role="status">{message}</p>}</section>;
}
