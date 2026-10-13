"use client";


import { networkUnavailable } from "@/lib/confirm";
import { defaultCopy, type CopyTranslator } from "@/lib/copy";
import { useTranslations } from "next-intl";
import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { AdminShell } from "../../shell";
import { ProfessionalCustomerList } from "@/components/professional-customer-list";
import { ChannelOnboardingPanel } from "@/components/channel-onboarding";
import { customerReturnHref } from "@/lib/customer";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { ChannelAdminsPanel } from "../admins-panel";
import { ChannelQuotaPanel } from "../quota-panel";
import { ChannelPnLPanel } from "../pnl-panel";
import { AdminSupplierPanel } from "../../billing/supplier-panel";
import { ChannelModelsPanel } from "../models-panel";
import { ChannelPaymentReadiness } from "../payment-readiness";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { STATUS_OPTIONS, channelTypeLabel } from "@/lib/tenants";
import { IfCan } from "@/components/rbac/if-can";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string; parent_id?: string };
type Role = { id: string; channel_org_id: string; type: string; parent_id?: string; status: string };
type ItemResponse = { item?: Channel; error?: { message?: string } };

function makePatchSchema(tUI: CopyTranslator) { return z.object({
  status: z.string().trim().min(1, tUI("consoleCopy.selectAStatus")),
}); }

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

export default function AdminChannelDetailPage() {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);

  const patchSchema = makePatchSchema(tUI);

  const tc = useTranslations("common");
  const router = useRouter();
  const viewer=useViewer();const brand=useBrand();const scope=`${viewer.userId || ""}:${brand?.id || ""}`;
  const contextParams=useSearchParams();
  const params = useParams<{ id: string }>();
  const raw = params.id;
  const id = decodeURIComponent(Array.isArray(raw) ? raw[0] : raw || "");
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState(tUI("consoleCopy.disablingFreezesNewUsageWhileRetainingBalanceAnd"));
  const query = useQuery({
    queryKey: ["/admin/channels", id,scope,viewer.roles.join(",")],
    enabled: !viewer.loading,
    queryFn: () => apiClient<ItemResponse>("GET", `/admin/channels/${id}`),
  });
  const item = query.data?.item;
  useEffect(()=>{setEditing(false);setMessage("");},[scope,id]);
  useEffect(() => { if (item?.type === "C") router.replace(`/admin/oem-deliveries/${encodeURIComponent(id)}${contextParams.size?`?${contextParams}`:""}`); }, [item?.type, id, router, contextParams]);
  const managedByPlatform = !!item && ( !item.parent_id || item.parent_id === "chn_official_a");
  const form = useForm<z.infer<ReturnType<typeof makePatchSchema>>>({
    resolver: zodResolver(patchSchema),
    values: {
      status: item?.status || "active",
    },
  });

  if(query.isPending) return <p role="status">{tc("listLoading")}</p>;
  if(query.isError || !item) return <div role="alert"><p>{tc("listFailed")}</p><Button variant="outline" onClick={()=>void query.refetch()}>{tc("listRetry")}</Button></div>;
  return (
    <AdminShell>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link href={customerReturnHref(contextParams.get("return_to"),"/admin/channels",scope)} className="text-sm text-brand-emphasis no-underline hover:underline">
            {tUI("public.article.primary")}</Link>
          <h1 className="mt-3 text-lg font-semibold tracking-tight">{tUI("consoleCopy.channelDetails")}</h1>
          <p className="mt-1 text-sm text-ink-secondary">{item ? `${item.code} · ${channelTypeLabel(item.type, tUI)}` : id}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {managedByPlatform ? <IfCan action="channels.write">
          {editing ? (
            <>
              <Button size="sm" variant="outline" onClick={() => setEditing(false)}>
                {tUI("common.cancel")}</Button>
              <ConfirmButton
                size="sm"
                title={tUI("consoleCopy.confirmChannelChanges")}
                description={tUI("consoleCopy.disablingFreezesNewUsageWhileRetainingFinancialHistory")}
                validate={() => form.trigger()}
                onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                  try {
                  const res = await fetch(`${apiBase}/admin/channels/${id}`, {
                    method: "PATCH",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: JSON.stringify(values),
                  });
                  const body = await res.json();
                  if (!res.ok) {
                    setMessage(body.error?.message || tUI("user.saveFail"));
                    return false;
                  }
                  setMessage(tUI("consoleCopy.saved", { value0: String(body.item?.id), value1: String(body.item?.status), value2: String(body.item?.type) }));
                  setEditing(false);
                  await queryClient.invalidateQueries({ queryKey: ["/admin/channels", id] });
                  return true;
                  } catch {
                    setMessage(confirmNetworkUnavailable);
                    return false;
                  }
})}
              >
                {tUI("consoleCopy.saveChannel")}</ConfirmButton>
            </>
          ) : (
            <Button size="sm" onClick={() => setEditing(true)}>
              {tUI("user.edit")}</Button>
          )}
          </IfCan> : null}
        </div>
      </div>
      {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}
      <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
        <h3 className="mb-3 text-lg font-medium">{tUI("consoleCopy.channelInformation")}</h3>
        {editing ? (
          <Form {...form}>
            <form className="grid max-w-xl gap-3" onSubmit={(event) => event.preventDefault()}>
              <p className="text-sm text-ink-secondary">{tUI("consoleCopy.channelTypeAndBrandAreFixedAfterCreation")}</p>
              <FormField
                control={form.control}
                name="status"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{tUI("compareUi.status")}</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label={tUI("consoleCopy.channelStatus")} {...field}>
                        {STATUS_OPTIONS.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </form>
          </Form>
        ) : (
          <dl className="grid max-w-xl gap-2 text-sm">
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">{tUI("consoleCopy.channelId")}</dt>
              <dd className="font-mono">{item?.id || id}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">{tUI("channelUi.bCodeLabel")}</dt>
              <dd>{item?.code || "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">{tUI("consoleCopy.channelType")}</dt>
              <dd>{item ? channelTypeLabel(item.type, tUI) : "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">{tUI("compareUi.status")}</dt>
              <dd>{item?.status || "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">{tUI("channelNav.brand")}</dt>
              <dd>{item?.brand_id || "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 py-2">
              <dt className="text-ink-secondary">{tUI("consoleCopy.parentChannel")}</dt>
              <dd>{item?.parent_id || tUI("user.examplesNone")}</dd>
            </div>
          </dl>
        )}
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      {item?.type === "B" ? <ChannelOnboardingPanel channelID={id} /> : null}
      {item && (item.id === "chn_official_a" || !item.parent_id || item.parent_id === "chn_official_a") ? <IfCan action="models.grant">
        <ChannelModelsPanel channelID={id} />
      </IfCan> : null}
      <IfCan action="channels.quota">
        {item?.type === "C" ? <ChannelQuotaPanel channelID={id} channelType={item.type} /> : null}
      </IfCan>
      {item?.type === "A" ? <><ChannelPnLPanel channelID={id} /><AdminSupplierPanel channelID={id} /></> : null}
      {item && managedByPlatform && item.type !== "A" ? <IfCan action="channels.write"><ChannelAdminsPanel channelID={id} code={item.code} /></IfCan> : null}
      {item?.type === "C" ? <ChannelPaymentReadiness channelID={id} /> : null}
      <IfCan action="partners.view"><ProfessionalCustomerList channelID={id}/></IfCan>
    </AdminShell>
  );
}
