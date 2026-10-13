"use client";

import { networkUnavailable } from "@/lib/confirm";
import { catalogLabels } from "@/lib/catalog-copy";
import { providerStatuses } from "@/lib/catalog-admin";
import { defaultCopy, type CopyTranslator } from "@/lib/copy";
import { useEffect, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { safeReturnHref } from "@/lib/return-context";
import { useViewer } from "@/components/rbac/viewer-context";
import { useParams, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { AdminShell } from "../../shell";
import { ProbeCell } from "../probe-cell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { modelEditHref, vendorLabel } from "@/lib/catalog";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";
import { CATALOG_LABEL } from "@/lib/catalog-copy";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import {
  PROVIDER_STATUSES,
  adapterLabel,
  protocolOptions,
  catalogStatusTone,
  healthLabel,
  healthTone,
  modelStatusLabel,
  providerKindLabel,
  providerStatusLabel,
  type MappedPublicModel,
} from "@/lib/catalog-admin";

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

type Provider = {
  id: string;
  name: string;
  slug: string;
  kind?: string;
  adapter: string;
  base_url?: string;
  health: string;
  status: string;
  timeout_ms?: number;
  health_checked_at?: string;
  models?: MappedPublicModel[];
};

type ItemResponse = { item?: Provider; error?: { message?: string } };
type Account = { id: string; label: string; fingerprint: string; status: string; kind: string };

function makePatchSchema(tUI: CopyTranslator) { return z.object({
  name: z.string().trim().min(1, tUI("consoleCopy.enterAName")),
  kind: z.enum(["direct", "aggregator"]),
  adapter: z.string().trim().min(1, tUI("consoleCopy.selectAProtocol")),
  base_url: z.string().trim(),
  status: z.string().trim().min(1, tUI("consoleCopy.selectAStatus")),
  timeout_ms: z.string().trim(),
}); }

function makeAddAccountSchema(tUI: CopyTranslator) { return z.object({
  label: z.string().trim().min(1, tUI("consoleCopy.enterALabel")),
  secret: z.string().min(1, tUI("consoleCopy.enterTheAccountSecret")),
}); }

function accountKindLabel(kind?: string, tUI: CopyTranslator = defaultCopy): string {
  switch ((kind || "").trim()) {
    case "adapter_secret":
      return tUI("consoleCopy.adapterCredential");
    case "api_key":
      return "API Key";
    default:
      return kind?.trim() || "—";
  }
}

export default function AdminProviderDetailPage() {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);
  const PROVIDER_STATUSES = providerStatuses(tUI);
  const patchSchema = makePatchSchema(tUI);

  const t = useTranslations("modelService");
  const viewer = useViewer();
  const searchParams = useSearchParams();
  const originModel = searchParams.get("model");
  const returnTo = searchParams.get("return_to");
  const params = useParams<{ id: string }>();
  const raw = params.id;
  const routeID = decodeURIComponent(Array.isArray(raw) ? raw[0] : raw || "");
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("");
  const query = useQuery({
    queryKey: [viewer.userId, "/admin/providers", routeID],
    queryFn: () => apiClient<ItemResponse>("GET", `/admin/providers/${encodeURIComponent(routeID)}`),
  });
  const item = query.data?.item;
  const providerID = item?.id || "";
  const form = useForm<z.infer<ReturnType<typeof makePatchSchema>>>({
    resolver: zodResolver(patchSchema),
    values: {
      name: item?.name || "",
      kind: item?.kind === "aggregator" ? "aggregator" : "direct",
      adapter: item?.adapter || "openai",
      base_url: item?.base_url || "",
      status: item?.status || "active",
      timeout_ms: item?.timeout_ms ? String(item.timeout_ms) : "",
    },
  });

  return (
    <AdminShell>
      {originModel ? <Link className="text-brand-emphasis underline" href={safeReturnHref(returnTo, modelEditHref(originModel))}>{t("returnModel")}</Link> : null}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link href="/admin/providers" className="text-sm text-brand-emphasis no-underline hover:underline">
            {tUI("public.article.primary")}</Link>
          <h1 className="mt-3 text-lg font-semibold tracking-tight">{tUI("consoleCopy.providerDetails")}</h1>
          <p className="mt-1 text-sm text-ink-secondary">
            {item ? `${item.name} · ${providerKindLabel(item.kind, tUI)} · ${adapterLabel(item.adapter, tUI)}` : routeID}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {item ? <ProbeCell id={item.id} /> : null}
          <IfCan action="providers.write">
            {editing ? (
              <>
                <Button size="sm" variant="outline" onClick={() => setEditing(false)}>
                  {tUI("common.cancel")}</Button>
                <ConfirmButton
                  size="sm"
                  title={tUI("consoleCopy.confirmProviderChanges")}
                  description={t("providerHint")}
                  validate={() => form.trigger()}
                  onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                    try {
                    const payload: Record<string, unknown> = {
                      name: values.name,
                      kind: values.kind,
                      adapter: values.adapter,
                      status: values.status,
                    };
                    if (values.base_url) {
                      payload.base_url = values.base_url;
                    }
                    if (values.timeout_ms) {
                      payload.timeout_ms = Number(values.timeout_ms);
                    }
                    const res = await fetch(`${apiBase}/admin/providers/${providerID}`, {
                      method: "PATCH",
                      credentials: "include",
                      headers: confirmHeaders,
                      body: JSON.stringify(payload),
                    });
                    const body = await res.json();
                    if (!res.ok) {
                      setMessage(body.error?.message || tUI("user.saveFail"));
                      return false;
                    }
                    setMessage(tUI("consoleCopy.saved259", { value0: String(body.item?.slug || body.item?.id), value1: String(providerStatusLabel(body.item?.status, tUI)) }));
                    setEditing(false);
                    await queryClient.invalidateQueries({ queryKey: [viewer.userId, "/admin/providers", routeID] });
                    return true;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
})}
                >
                  {tUI("consoleCopy.saveProvider")}</ConfirmButton>
              </>
            ) : (
              <Button size="sm" onClick={() => setEditing(true)}>
                {tUI("user.edit")}</Button>
            )}
          </IfCan>
        </div>
      </div>
      {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}

      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="editProvider" className="mb-3 text-lg font-semibold tracking-tight" />
        {editing ? (
          <Form {...form}>
            <form className="grid max-w-xl gap-3" onSubmit={(event) => event.preventDefault()}>
              <p className="text-sm text-ink-secondary">
                {tUI("consoleCopy.changeStatusProtocolAndUpstreamUrlKeepThe")}</p>
              <TextField control={form.control} name="name" label={tUI("user.colName")} />
              <FormField
                control={form.control}
                name="kind"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{tUI("partnerBoard.colKind")}</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label={tUI("partnerBoard.colKind")} {...field}>
                        <option value="direct">{tUI("consoleCopy.directOfficialOrCompatibleProtocol")}</option>
                        <option value="aggregator">{tUI("consoleCopy.aggregatorOpenrouterAndOthersNoRoutingBackTo")}</option>
                      </select>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="adapter"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{tUI("quickstartUi.statProto")}</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label={tUI("quickstartUi.statProto")} {...field}>
                        {protocolOptions(field.value, tUI).map((option) => (
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
              <TextField control={form.control} name="base_url" label={tUI("consoleCopy.upstreamUrl")} placeholder="https://api.openai.com/v1" />
              <FormField
                control={form.control}
                name="status"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{tUI("compareUi.status")}</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label={tUI("compareUi.status")} {...field}>
                        {PROVIDER_STATUSES.map((option) => (
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
              <TextField control={form.control} name="timeout_ms" label={tUI("consoleCopy.upstreamTimeoutMilliseconds")} placeholder={tUI("consoleCopy.default30000")} />
            </form>
          </Form>
        ) : (
          <dl className="grid max-w-3xl gap-3 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-ink-secondary">{tUI("consoleCopy.health")}</dt>
              <dd className="mt-1">
                <Badge tone={healthTone(item?.health)}>{item?.health_checked_at && Date.now() - new Date(item.health_checked_at).getTime() <= 86400000 ? healthLabel(item.health, tUI) : t("state.unknown")}</Badge>
              </dd>
            </div>
            <div>
              <dt className="text-ink-secondary">{tUI("compareUi.status")}</dt>
              <dd className="mt-1">
                <Badge tone={catalogStatusTone(item?.status)}>{providerStatusLabel(item?.status, tUI)}</Badge>
              </dd>
            </div>
            <div>
              <dt className="text-ink-secondary">{tUI("consoleCopy.upstreamTimeout")}</dt>
              <dd className="mt-1">{item?.timeout_ms ? `${item.timeout_ms} ms` : tUI("consoleCopy.default30000Ms")}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-ink-secondary">{tUI("consoleCopy.upstreamUrl")}</dt>
              <dd className="mt-1 font-mono text-[13px]">{item?.base_url || tUI("consoleCopy.notSetOptionalForSandbox")}</dd>
            </div>
          </dl>
        )}
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>

      <AccountPoolPanel providerID={providerID} />
      {providerID ? <ProviderModelsPanel providerID={providerID} /> : null}
      <MappedModelsPanel models={item?.models || []} />
    </AdminShell>
  );
}

type UpstreamModel = {
  upstream_model_id: string;
  display_name: string;
  unit_costs: Record<string, string>;
  price_source: string;
  status: string;
};

function costSummary(costs: Record<string, string>, tUI: CopyTranslator = defaultCopy) {
  const parts: string[] = [];
  if (costs.input) parts.push(tUI("consoleCopy.inputUsdMillion", { value0: String(perTokenToPerMillion(costs.input)) }));
  if (costs.output) parts.push(tUI("consoleCopy.outputUsdMillion", { value0: String(perTokenToPerMillion(costs.output)) }));
  if (costs.image_count) parts.push(tUI("consoleCopy.imageUsdImage", { value0: String(costs.image_count) }));
  if (costs.video_second) parts.push(tUI("consoleCopy.videoUsdSecond", { value0: String(costs.video_second) }));
  if (costs.audio_second) parts.push(tUI("consoleCopy.audioUsdSecond", { value0: String(costs.audio_second) }));
  return parts.join(" · ") || tUI("consoleCopy.priceMissing");
}

function ProviderModelsPanel({ providerID }: { providerID: string }) {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);

  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const [search, setSearch] = useState("");
  const [editingID, setEditingID] = useState<string | null>(null);
  const [modelID, setModelID] = useState("");
  const [input, setInput] = useState("");
  const [output, setOutput] = useState("");
  const [image, setImage] = useState("");
  const [video, setVideo] = useState("");
  const [audio, setAudio] = useState("");
  const queryKey = ["/admin/providers", providerID, "upstream-models"];
  const query = useQuery({
    queryKey,
    queryFn: () => apiClient<{ items?: UpstreamModel[] }>("GET", `/admin/providers/${providerID}/upstream-models`),
  });
  const models = (query.data?.items ?? []).filter((item) =>
    `${item.upstream_model_id} ${item.display_name}`.toLowerCase().includes(search.toLowerCase()));

  function edit(item?: UpstreamModel) {
    setEditingID(item?.upstream_model_id ?? "");
    setModelID(item?.upstream_model_id ?? "");
    setInput(perTokenToPerMillion(item?.unit_costs.input ?? ""));
    setOutput(perTokenToPerMillion(item?.unit_costs.output ?? ""));
    setImage(item?.unit_costs.image_count ?? "");
    setVideo(item?.unit_costs.video_second ?? "");
    setAudio(item?.unit_costs.audio_second ?? "");
    setMessage("");
  }

  async function discover() {
    try {
      const res = await fetch(`${apiBase}/admin/providers/${providerID}/upstream-models/discover`, {
        method: "POST", credentials: "include", headers: confirmHeaders,
      });
      const body = await res.json();
      if (!res.ok) { setMessage(body.error?.message || tUI("consoleCopy.thisUpstreamDoesNotSupportModelDiscoveryAdd")); return; }
      setMessage(tUI("consoleCopy.discoveredUpstreamModelsPricesMissingFromTheResponse", { value0: String(body.items?.length ?? 0) }));
      await queryClient.invalidateQueries({ queryKey });
    } catch { setMessage(confirmNetworkUnavailable); }
  }

  async function save() {
    try {
      const unitCosts = { ...millionDim(input, output, tUI) } as Record<string, string>;
      if (image.trim()) unitCosts.image_count = image.trim();
      if (video.trim()) unitCosts.video_second = video.trim();
      if (audio.trim()) unitCosts.audio_second = audio.trim();
      const res = await fetch(`${apiBase}/admin/providers/${providerID}/upstream-models`, {
        method: "PUT", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ upstream_model_id: modelID.trim(), unit_costs: unitCosts }),
      });
      const body = await res.json();
      if (!res.ok) { setMessage(body.error?.message || tUI("consoleCopy.couldNotSaveTheCostPrice")); return; }
      setEditingID(null);
      setMessage(tUI("consoleCopy.upstreamModelCostPriceSaved"));
      await queryClient.invalidateQueries({ queryKey });
    } catch (err) { setMessage(err instanceof Error ? err.message : confirmNetworkUnavailable); }
  }

  async function setEnabled(item: UpstreamModel, enabled: boolean) {
    try {
      const res = await fetch(`${apiBase}/admin/providers/${providerID}/upstream-models/status`, {
        method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ upstream_model_id: item.upstream_model_id, enabled }),
      });
      const body = await res.json();
      if (!res.ok) { setMessage(body.error?.message || tUI("consoleCopy.statusUpdateFailed")); return false; }
      setMessage(enabled ? tUI("consoleCopy.upstreamModelEnabled") : tUI("consoleCopy.upstreamModelDisabledFutureRequestsWillNotSelect"));
      await queryClient.invalidateQueries({ queryKey });
      await queryClient.invalidateQueries({ queryKey: ["/admin/routes"] });
      return true;
    } catch { setMessage(confirmNetworkUnavailable); return false; }
  }

  const fieldClass = "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm";
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6">
    <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h3 className="text-lg font-semibold">{tUI("consoleCopy.supportedModelsAndPrices")}</h3>
      <IfCan action="providers.write"><div className="flex gap-2">
        <Button size="sm" variant="outline" onClick={discover}>{tUI("consoleCopy.discoverUpstreamModels")}</Button>
        <Button size="sm" variant="outline" onClick={() => edit()}>{tUI("consoleCopy.addManually")}</Button>
      </div></IfCan>
    </div>
    <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.readPricesAutomaticallyWhenSuppliedByTheUpstream")}</p>
    <input className={`${fieldClass} mb-3 max-w-sm`} aria-label={tUI("consoleCopy.searchUpstreamModels")} placeholder={tUI("consoleCopy.searchUpstreamModels")} value={search} onChange={(event) => setSearch(event.target.value)} />
    <div className="max-h-96 overflow-auto">
      {models.length === 0 ? <p className="py-3 text-sm text-ink-secondary">{tUI("consoleCopy.noUpstreamModelsYetDiscoverModelsOrAdd")}</p> :
        <table className="min-w-full text-left text-sm"><thead><tr className="border-b border-hairline text-ink-secondary">
          <th className="px-2 py-2">{tUI("consoleCopy.upstreamModel")}</th><th className="px-2 py-2">{tUI("adminUi.colUpstream")}</th><th className="px-2 py-2">{tUI("compareUi.status")}</th><th className="px-2 py-2">{tUI("blogUi.source")}</th><th className="px-2 py-2">{tUI("user.colActions")}</th>
        </tr></thead><tbody>{models.map((item) => <tr key={item.upstream_model_id} className="border-b border-hairline/80">
          <td className="px-2 py-2"><span className="font-mono">{item.upstream_model_id}</span>{item.display_name && item.display_name !== item.upstream_model_id ? <span className="ml-2 text-ink-secondary">{item.display_name}</span> : null}</td>
          <td className="px-2 py-2 font-mono">{costSummary(item.unit_costs, tUI)}</td>
          <td className="px-2 py-2">{item.status === "inactive" ? tUI("common.stDisabled") : tUI("common.stActive")}</td>
          <td className="px-2 py-2">{item.price_source === "manual" ? tUI("consoleCopy.manual") : item.price_source === "detected" ? tUI("consoleCopy.upstream") : tUI("consoleCopy.priceMissing")}</td>
          <td className="px-2 py-2"><IfCan action="providers.write"><div className="flex gap-1"><Button size="sm" variant="ghost" onClick={() => edit(item)}>{tUI("consoleCopy.editPrice")}</Button><ConfirmButton size="sm" variant="ghost" title={item.status === "inactive" ? tUI("consoleCopy.enableUpstreamModel") : tUI("consoleCopy.disableUpstreamModel")} description={item.status === "inactive" ? tUI("consoleCopy.enablingMakesItEligibleForMatchingRoutesAgain") : tUI("consoleCopy.newRequestsWillNoLongerSelectThisUpstream")} onConfirm={() => setEnabled(item, item.status === "inactive")}>{item.status === "inactive" ? tUI("keyUX.enable") : tUI("staff.disable")}</ConfirmButton></div></IfCan></td>
        </tr>)}</tbody></table>}
    </div>
    {editingID !== null ? <div className="mt-4 grid max-w-3xl gap-3 rounded-control border border-hairline p-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm">{tUI("consoleCopy.upstreamModelIdUsedForCalls")}<input className={fieldClass} value={modelID} disabled={!!editingID} onChange={(event) => setModelID(event.target.value)} /></label>
        <label className="text-sm">{tUI("consoleCopy.inputCostUsdMillionTokens")}<input className={fieldClass} value={input} onChange={(event) => setInput(event.target.value)} /></label>
        <label className="text-sm">{tUI("consoleCopy.outputCostUsdMillionTokens")}<input className={fieldClass} value={output} onChange={(event) => setOutput(event.target.value)} /></label>
        <label className="text-sm">{tUI("consoleCopy.imageCostUsdImage")}<input className={fieldClass} value={image} onChange={(event) => setImage(event.target.value)} /></label>
        <label className="text-sm">{tUI("consoleCopy.videoCostUsdSecond")}<input className={fieldClass} value={video} onChange={(event) => setVideo(event.target.value)} /></label>
        <label className="text-sm">{tUI("consoleCopy.audioCostUsdSecond")}<input className={fieldClass} value={audio} onChange={(event) => setAudio(event.target.value)} /></label>
      </div>
      <div className="flex gap-2"><Button size="sm" onClick={save}>{tUI("consoleCopy.savePrice")}</Button><Button size="sm" variant="outline" onClick={() => setEditingID(null)}>{tUI("common.cancel")}</Button></div>
    </div> : null}
    {message ? <p className="mt-3 text-sm text-ink-secondary">{message}</p> : null}
  </section>;
}

function MappedModelsPanel({ models }: { models: MappedPublicModel[] }) {
  const tUI = useTranslations();

  const CATALOG_LABEL = catalogLabels(tUI);
  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <AdminH2 k="mappedModels" className="mb-3 text-lg font-semibold tracking-tight" />
      <p className="mb-3 text-sm text-ink-secondary">
        {tUI("consoleCopy.readOnlyViewOfModelsAndUpstreamIds")}</p>
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="th-eyebrow px-2 py-2 font-medium">{CATALOG_LABEL.publicModelId}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{CATALOG_LABEL.vendor}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{CATALOG_LABEL.upstreamModelId}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{tUI("compareUi.status")}</th>
            </tr>
          </thead>
          <tbody>
            {models.length === 0 ? (
              <tr>
                <td className="px-2 py-3 text-ink-secondary" colSpan={4}>
                  {tUI("consoleCopy.noPublicModelsConnectedYetPublishAModel")}</td>
              </tr>
            ) : (
              models.map((model) => (
                <tr key={`${model.public_id}-${model.upstream_model_id}`} className="border-b border-hairline/80">
                  <td className="px-2 py-2">
                    {model.public_id ? (
                      <Link className="font-mono text-[13px] text-brand-emphasis no-underline hover:underline" href={modelEditHref(model.public_id)}>
                        {model.public_id}
                      </Link>
                    ) : (
                      "—"
                    )}
                    {model.display_name ? <span className="ml-2 text-ink-secondary">{model.display_name}</span> : null}
                  </td>
                  <td className="px-2 py-2">{vendorLabel(model.vendor, tUI)}</td>
                  <td className="px-2 py-2 font-mono text-[13px]">{model.upstream_model_id || "—"}</td>
                  <td className="px-2 py-2">
                    <Badge tone={catalogStatusTone(model.status)}>{modelStatusLabel(model.status, tUI)}</Badge>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function AccountPoolPanel({ providerID }: { providerID: string }) {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);

  const addAccountSchema = makeAddAccountSchema(tUI);

  const queryClient = useQueryClient();
  const [items, setItems] = useState<Account[]>([]);
  const [message, setMessage] = useState(tUI("consoleCopy.loadingAccounts"));
  const [loading, setLoading] = useState(false);
  const addForm = useForm<z.infer<ReturnType<typeof makeAddAccountSchema>>>({
    resolver: zodResolver(addAccountSchema),
    defaultValues: { label: "primary", secret: "" },
  });

  async function load(id = providerID) {
    if (!id) {
      return;
    }
    setLoading(true);
    try {
      const res = await fetch(`${apiBase}/admin/providers/${encodeURIComponent(id)}/accounts`, { credentials: "include" });
      const body = await res.json();
      if (!res.ok) {
        setMessage(body.error?.message || tUI("consoleCopy.couldNotLoadAccounts"));
        setItems([]);
        return;
      }
      const raw = JSON.stringify(body);
      if (raw.includes("ciphertext") || raw.includes("\"secret\"")) {
        setMessage(tUI("consoleCopy.theAccountResponseExposedASecretAndWas"));
        setItems([]);
        return;
      }
      setItems(body.items || []);
      setMessage(tUI("consoleCopy.loadedAccounts", { value0: String(body.items?.length ?? 0) }));
    } catch {
      setMessage(confirmNetworkUnavailable);
      setItems([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!providerID) {
      return;
    }
    void load(providerID);
    // load is local to this render; providerID is the trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providerID]);

  async function patch(accountID: string, payload: Record<string, unknown>, okText: string): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/admin/providers/${providerID}/accounts/${accountID}`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify(payload),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || tUI("consoleCopy.updateFailed"));
      return false;
    }
    await load();
    setMessage(`${okText} ${body.item?.fingerprint || accountID} → ${body.item?.status}`);
    await queryClient.invalidateQueries();
    return true;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <AdminH2 k="accountPool" className="mb-3 text-lg font-semibold tracking-tight" />
      <p className="mb-3 text-sm text-ink-secondary">
        {tUI("consoleCopy.aProviderCanHaveMultipleUpstreamKeysRouting")}</p>
      <p role="status" className="mb-3 text-sm text-ink-secondary">{message}</p>
      <div className="mb-3 overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="th-eyebrow px-2 py-2 font-medium">{tUI("consoleCopy.label")}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{tUI("consoleCopy.fingerprint")}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{tUI("consoleCopy.kind")}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{tUI("compareUi.status")}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{tUI("user.colActions")}</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-2 py-3 text-ink-secondary">
                  {loading ? tUI("consoleCopy.loadingAccounts") : tUI("consoleCopy.noAccountsYetAddOneBelow")}
                </td>
              </tr>
            ) : items.map((row) => (
              <tr key={row.id} className="border-b border-hairline/80">
                <td className="px-2 py-2">{row.label}</td>
                <td className="px-2 py-2 font-mono text-[13px]">{row.fingerprint}</td>
                <td className="px-2 py-2">{accountKindLabel(row.kind, tUI)}</td>
                <td className="px-2 py-2">
                  <Badge tone={catalogStatusTone(row.status)}>{modelStatusLabel(row.status, tUI)}</Badge>
                </td>
                <td className="px-2 py-2">
                  <div className="flex flex-wrap gap-2">
                    <IfCan action="providers.write">
                      <ConfirmButton size="sm" variant="outline" title={tUI("consoleCopy.confirmAccountCooldown")} description={tUI("consoleCopy.anAccountOnCooldownWillNotBeSelected")} onConfirm={() => patch(row.id, { cooldown_seconds: 120 }, tUI("consoleCopy.onCooldown"))}>
                        {tUI("consoleCopy.coolDown")}</ConfirmButton>
                      <ConfirmButton size="sm" variant="outline" title={tUI("consoleCopy.confirmAccountDisabling")} description={tUI("consoleCopy.aDisabledAccountWillNotBeSelectedFor")} onConfirm={() => patch(row.id, { status: "disabled" }, tUI("common.stDisabled"))}>
                        {tUI("staff.disable")}</ConfirmButton>
                    </IfCan>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <IfCan action="providers.write">
        <Form {...addForm}>
          <form className="grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <TextField control={addForm.control} name="label" label={tUI("consoleCopy.accountLabel")} placeholder="primary" />
            <TextField control={addForm.control} name="secret" label={tUI("consoleCopy.accountSecret")} type="password" autoComplete="new-password" />
            <ConfirmButton
              size="sm"
              title={tUI("consoleCopy.confirmAccountAddition")}
              description={tUI("consoleCopy.onlyFingerprintsAppearInTheListSecretsAre")}
              validate={() => addForm.trigger()}
              onConfirm={confirmFormSubmit(addForm.handleSubmit, async (values) => {
                try {
                const res = await fetch(`${apiBase}/admin/providers/${providerID}/accounts`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({ secret: values.secret, label: values.label, kind: "api_key" }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || tUI("consoleCopy.additionFailed"));
                  return false;
                }
                if (body.item?.secret || body.item?.ciphertext) {
                  setMessage(tUI("consoleCopy.theAddAccountResponseExposedASecret"));
                  return false;
                }
                addForm.reset({ label: "primary", secret: "" });
                await load(providerID);
                setMessage(tUI("consoleCopy.addedAccountFingerprint", { value0: String(body.item?.fingerprint || "") }));
                return true;
                } catch {
                  setMessage(confirmNetworkUnavailable);
                  return false;
                }
})}
            >
              {tUI("consoleCopy.addAccount")}</ConfirmButton>
          </form>
        </Form>
      </IfCan>
    </section>
  );
}
