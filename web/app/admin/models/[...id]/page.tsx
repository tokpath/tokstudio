"use client";


import { networkUnavailable } from "@/lib/confirm";
import { defaultCopy, type CopyTranslator } from "@/lib/copy";
import { useTranslations } from "next-intl";
import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { AdminSelectField } from "@/components/admin-select-field";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { SealConfirm } from "@/components/seal-confirm";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { IfCan } from "@/components/rbac/if-can";
import { AdminListPanel } from "../../list-panel";
import { AdminShell } from "../../shell";
import { apiBase } from "@/lib/api";
import { safeReturnHref } from "@/lib/return-context";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { ModelServicePanel } from "../service-readiness";
import { useViewer } from "@/components/rbac/viewer-context";
import { type AdminModel, modelEditHref } from "@/lib/catalog";
import { catalogStatusTone, modelStatusLabel } from "@/lib/catalog-admin";
import { priceBookColumns, publishedPriceLabel, type PriceBook } from "@/lib/price-book";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";

function makeKinds(tUI: CopyTranslator) { return [
  { value: "text", label: tUI("home.tabText") }, { value: "image", label: tUI("consoleCopy.image") },
  { value: "video", label: tUI("home.tabVideo") }, { value: "audio", label: tUI("consoleCopy.audio") },
  { value: "embedding", label: tUI("catalog.embedding") },
]; }
function makeModelSchema(tUI: CopyTranslator) { return z.object({
  display_name: z.string().trim().min(1, tUI("consoleCopy.enterAName")),
  vendor: z.string().trim().min(1, tUI("consoleCopy.enterAVendor")),
  kind: z.enum(["text", "image", "video", "audio", "embedding"]),
  description: z.string(),
}); }
const priceSchema = z.object({
  input: z.string(), output: z.string(), image_count: z.string(), video_second: z.string(), audio_second: z.string(),
  currency: z.string().min(1),
});
type ModelFields = z.infer<ReturnType<typeof makeModelSchema>>;
type PriceFields = z.infer<typeof priceSchema>;

function mergedCapabilities(model: AdminModel | undefined, values: ModelFields): Record<string, unknown> {
  const caps = { ...(model?.capabilities ?? {}) };
  caps.kind = values.kind;
  caps.description = values.description.trim();
  return caps;
}

function pricePayload(values: PriceFields, tUI: CopyTranslator = defaultCopy): Record<string, unknown> {
  const payload: Record<string, unknown> = { currency: values.currency };
  const sell = millionDim(values.input, values.output, tUI);
  if (sell) payload.customer_sell = sell;
  for (const key of ["image_count", "video_second", "audio_second"] as const) {
    if (values[key].trim()) payload[key] = values[key].trim();
  }
  return payload;
}

export default function AdminModelEditPage() {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);

  const modelSchema = makeModelSchema(tUI);
  const kinds = makeKinds(tUI);

  const viewer = useViewer();
  const returnTo = useSearchParams().get("return_to");
  const params = useParams<{ id?: string | string[] }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const publicId = useMemo(() => Array.isArray(params.id) ? params.id.join("/") : params.id || "", [params.id]);
  const isNew = publicId === "new";
  const [message, setMessage] = useState("");
  const [priceMessage, setPriceMessage] = useState("");
  const [lifeError, setLifeError] = useState("");
  const query = useQuery({
    queryKey: [viewer.userId, "/admin/models", publicId],
    queryFn: () => apiClient<{ item?: AdminModel; error?: { message?: string } }>("GET", `/admin/models/${publicId}`),
    enabled: !!publicId && !isNew,
  });
  const model = query.data?.item;
  const modelForm = useForm<ModelFields>({
    resolver: zodResolver(modelSchema),
    defaultValues: { display_name: "", vendor: "", kind: "text", description: "" },
  });
  const priceForm = useForm<PriceFields>({
    resolver: zodResolver(priceSchema),
    defaultValues: {
      input: "", output: "", image_count: "", video_second: "", audio_second: "",
      currency: "USD",
    },
  });
  const kind = modelForm.watch("kind");

  useEffect(() => {
    if (!model) return;
    const caps = model.capabilities ?? {};
    const kindValue = typeof caps.kind === "string" ? caps.kind : model.kind;
    const storedKind = kinds.some((item) => item.value === kindValue) ? kindValue as ModelFields["kind"] : "text";
    modelForm.reset({
      display_name: model.display_name, vendor: model.vendor,
      kind: storedKind, description: typeof caps.description === "string" ? caps.description : "",
    });
    priceForm.reset({
      input: perTokenToPerMillion(String(model.sell_price?.input ?? "")),
      output: perTokenToPerMillion(String(model.sell_price?.output ?? "")),
      image_count: String(model.sell_price?.image_count ?? ""),
      video_second: String(model.sell_price?.video_second ?? ""),
      audio_second: String(model.sell_price?.audio_second ?? ""),
      currency: "USD",
    });
  }, [model, modelForm, priceForm]);

  async function reload() {
    await query.refetch();
    await queryClient.invalidateQueries({ predicate: query => query.queryKey.includes("/admin/models") });
  }

  async function saveModel(values: ModelFields): Promise<boolean> {
    try {
      const initialPrice = isNew ? pricePayload(priceForm.getValues()) : undefined;
      const res = await fetch(`${apiBase}/admin/models${isNew ? "" : `/${publicId}`}`, {
        method: isNew ? "POST" : "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({
          display_name: values.display_name, vendor: values.vendor,
          capabilities: mergedCapabilities(model, values),
          ...(initialPrice ? { initial_price: initialPrice } : {}),
        }),
      });
      const body = await res.json();
      if (!res.ok) { setMessage(body.error?.message || tUI("user.saveFail")); return false; }
      if (isNew && !body.item?.id) { setMessage(tUI("consoleCopy.modelCreatedButNoCallIdWasReturned")); return false; }
      setMessage(isNew ? tUI("consoleCopy.modelAndInitialPriceSaved") : tUI("consoleCopy.modelInformationSaved"));
      if (isNew) router.replace(modelEditHref(body.item.id));
      else await reload();
      return true;
    } catch (err) { setMessage(err instanceof Error ? err.message : confirmNetworkUnavailable); return false; }
  }

  async function validateNewModel(): Promise<boolean> {
    const [modelValid, priceValid] = await Promise.all([modelForm.trigger(), priceForm.trigger()]);
    if (!modelValid || !priceValid) return false;
    const values = priceForm.getValues();
    const required: (keyof PriceFields)[] = kind === "text" ? ["input", "output"] :
      kind === "embedding" ? ["input"] : kind === "image" ? ["image_count"] :
      kind === "video" ? ["video_second"] : ["audio_second"];
    let complete = true;
    for (const key of required) {
      const value = values[key].trim();
      if (!value || !/^\d+(?:\.\d+)?$/.test(value)) {
        priceForm.setError(key, { message: value ? tUI("consoleCopy.enterANonnegativeNumber") : tUI("consoleCopy.enterASellingPrice") });
        complete = false;
      }
    }
    return complete;
  }

  async function lifecycle(action: "publish" | "deprecate"): Promise<boolean> {
    try {
      const res = await fetch(`${apiBase}/admin/models/${action}`, {
        method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ public_id: publicId }),
      });
      const body = await res.json();
      if (!res.ok) { setLifeError(body.error?.message || tUI("user.actFail")); return false; }
      setLifeError("");
      await reload();
      return true;
    } catch { setLifeError(confirmNetworkUnavailable); return false; }
  }

  if (!isNew && (query.isPending || query.isError || query.data?.error || !model)) return <AdminShell>
      <h1 className="text-2xl font-semibold">{model?.display_name || publicId}</h1><section role={query.isPending ? "status" : "alert"}><p>{query.isPending ? tUI("consoleCopy.loadingModel") : tUI("consoleCopy.couldNotLoadTheModelConfigurationHasNot")}</p>{!query.isPending ? <Button variant="outline" onClick={()=>void query.refetch()}>{tUI("usageWorkflow.retry")}</Button>:null}</section></AdminShell>;
  return (
    <AdminShell>
      <h1 className="text-2xl font-semibold">{isNew ? tUI("adminUi.createModel") : model?.display_name || publicId}</h1>
      <Link className="text-sm text-brand-emphasis hover:underline" href={safeReturnHref(returnTo, "/admin/models")}>{tUI("consoleCopy.returnToModelList")}</Link>
      {query.data?.error ? <p className="text-sm text-danger">{query.data.error.message}</p> : null}

      {!isNew && model ? <ModelServicePanel model={model} /> : null}
      <IfCan action="models.write">
        <section className="rounded-card border border-hairline bg-canvas-raised p-6">
          <h3 className="text-base font-semibold">{tUI("consoleCopy.modelInformation")}</h3>
          <Form {...modelForm}>
            <form className="mt-4 grid max-w-2xl gap-4" onSubmit={(event) => event.preventDefault()}>
              {!isNew ? <p className="text-sm text-ink-secondary">{tUI("modelService.callID")}<code className="ml-2 font-mono text-ink-primary">{publicId}</code></p> : null}
              <TextField control={modelForm.control} name="display_name" label={tUI("user.colName")} />
              <TextField control={modelForm.control} name="vendor" label={tUI("modelService.vendor")} placeholder={tUI("consoleCopy.forExampleAlibaba")} />
              <AdminSelectField control={modelForm.control} name="kind" label={tUI("partnerBoard.colKind")} options={kinds} />
              <TextField control={modelForm.control} name="description" label={tUI("consoleCopy.description162")} />
              {!isNew ? <ConfirmButton
                size="sm" title={tUI("consoleCopy.confirmModelChanges")}
                description={tUI("consoleCopy.saveModelInformationThePublicModelIdCannot")}
                validate={() => modelForm.trigger()}
                onConfirm={confirmFormSubmit(modelForm.handleSubmit, saveModel)}
              >{tUI("consoleCopy.saveModel")}</ConfirmButton> : null}
              {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
            </form>
          </Form>
        </section>
      </IfCan>

      {(isNew || model) ? <>
        <IfCan action="prices.write">
          <section id="prices" className="scroll-mt-20 rounded-card border border-hairline bg-canvas-raised p-6">
            <h3 className="text-base font-semibold">{tUI("adminUi.colSell")}</h3>
            <p className="mt-1 text-sm text-ink-secondary">{isNew ? tUI("consoleCopy.theInitialPriceIsSavedWithTheModel") : tUI("consoleCopy.priceChangesApplyOnlyToFutureRequests")}</p>
            <Form {...priceForm}>
              <form className="mt-4 grid max-w-xl gap-3" onSubmit={(event) => event.preventDefault()}>
                {kind === "text" || kind === "embedding" ? <TextField control={priceForm.control} name="input" label={tUI("consoleCopy.inputPrice")} suffix={tUI("consoleCopy.usdMillionTokens")} /> : null}
                {kind === "text" ? <TextField control={priceForm.control} name="output" label={tUI("consoleCopy.outputPrice")} suffix={tUI("consoleCopy.usdMillionTokens")} /> : null}
                {kind === "image" ? <TextField control={priceForm.control} name="image_count" label={tUI("consoleCopy.pricePerImage")} suffix={tUI("consoleCopy.usdImage")} /> : null}
                {kind === "video" ? <TextField control={priceForm.control} name="video_second" label={tUI("consoleCopy.pricePerVideoSecond")} suffix={tUI("consoleCopy.usdSecond")} /> : null}
                {kind === "audio" ? <TextField control={priceForm.control} name="audio_second" label={tUI("consoleCopy.pricePerAudioSecond")} suffix={tUI("consoleCopy.usdSecond")} /> : null}
                {(kind === "image" || kind === "video" || kind === "audio") && model?.sell_price?.media &&
                  !model?.sell_price?.[kind === "image" ? "image_count" : kind === "video" ? "video_second" : "audio_second"] ?
                  <p className="text-sm text-ink-secondary">{tUI("consoleCopy.legacyPrice")}{String(model.sell_price.media)}{tUI("consoleCopy.mediaUnitWithNoCurrentBillingUnitSpecified")}</p> : null}
                <AdminSelectField control={priceForm.control} name="currency" label={tUI("consoleCopy.currency")} options={[{ value: "USD", label: tUI("consoleCopy.usDollarUsd") }]} />
                {isNew ? <ConfirmButton
                  size="sm" title={tUI("consoleCopy.confirmModelCreation")} description={tUI("consoleCopy.modelInformationAndTheInitialPriceWillBe")}
                  error={message} validate={validateNewModel}
                  onConfirm={confirmFormSubmit(modelForm.handleSubmit, saveModel)}
                >{tUI("adminUi.createModel")}</ConfirmButton> : <SealConfirm
                  size="sm" title={tUI("consoleCopy.confirmNewPricePublication")} description={tUI("consoleCopy.historicalBillsRetainTheirOriginalPriceSnapshots")}
                  validate={() => priceForm.trigger()}
                  onConfirm={confirmFormSubmit(priceForm.handleSubmit, async (values) => {
                    try {
                      const payload = { ...pricePayload(values, tUI), model: publicId };
                      const res = await fetch(`${apiBase}/admin/price-books`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify(payload) });
                      const body = await res.json();
                      if (!res.ok) { setPriceMessage(body.error?.message || tUI("consoleCopy.pricePublicationFailed")); return false; }
                      setPriceMessage(tUI("consoleCopy.published", { value0: String(publishedPriceLabel(body.price, publicId)) }));
                      await reload();
                      return true;
                    } catch (err) { setPriceMessage(err instanceof Error ? err.message : confirmNetworkUnavailable); return false; }
                  })}
                >{tUI("consoleCopy.publishPrice")}</SealConfirm>}
                {priceMessage ? <p className="text-sm text-ink-secondary">{priceMessage}</p> : null}
              </form>
            </Form>
          </section>
        </IfCan>
        {!isNew && model ? <IfCan action="models.write">
          <section className="rounded-card border border-hairline bg-canvas-raised p-6">
            <div className="flex flex-wrap items-center gap-3">
              <h3 className="text-base font-semibold">{tUI("consoleCopy.modelStatus")}</h3>
              <Badge tone={catalogStatusTone(model.status)}>{model.status === "published" && model.config_ready === false ? tUI("consoleCopy.configurationIncomplete") : modelStatusLabel(model.status, tUI)}</Badge>
            </div>
            <p className="mt-1 text-sm text-ink-secondary">{tUI("consoleCopy.aFullyConfiguredModelCanBeSelectedIn")}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              {model.status !== "published" ? <ConfirmButton
                size="sm" title={tUI("consoleCopy.confirmModelPublication")} description={tUI("consoleCopy.publishOnlyTheModelConfigurationNoRoutesOr")}
                error={lifeError} validate={() => {
                  if (modelForm.formState.isDirty || priceForm.formState.isDirty) {
                    setLifeError(tUI("consoleCopy.saveModelInformationOrPublishANewPrice"));
                    return false;
                  }
                  setLifeError("");
                  return true;
                }} onConfirm={() => lifecycle("publish")}
              >{tUI("modelService.missing.publish")}</ConfirmButton> : <>
                <ConfirmButton size="sm" variant="outline" title={tUI("consoleCopy.confirmModelDeprecation")} description={tUI("consoleCopy.deprecationHidesTheModelFromTheCustomerCatalog")} error={lifeError} onConfirm={() => lifecycle("deprecate")}>{tUI("adminUi.deprecateModel")}</ConfirmButton>
              </>}
            </div>
            {lifeError ? <p className="mt-3 text-sm text-danger">{lifeError}</p> : null}
          </section>
        </IfCan> : null}
        {!isNew && model ? <details className="rounded-card border border-hairline bg-canvas-raised p-6">
          <summary className="cursor-pointer text-sm font-medium">{tUI("consoleCopy.viewPriceVersions")}</summary>
          <div className="mt-4">
            <AdminListPanel<PriceBook>
              path={`/admin/price-books?q=${encodeURIComponent(publicId)}`}
              title={tUI("consoleCopy.priceVersions")} columns={priceBookColumns} emptyTitle={tUI("consoleCopy.noPriceVersionsYet")} emptyDetail={tUI("consoleCopy.publishingASellingPriceRecordsAVersion")}
            />
          </div>
        </details> : null}
      </> : null}
    </AdminShell>
  );
}
