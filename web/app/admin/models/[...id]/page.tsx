"use client";

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

const kinds = [
  { value: "text", label: "文本" }, { value: "image", label: "图片" },
  { value: "video", label: "视频" }, { value: "audio", label: "音频" },
  { value: "embedding", label: "向量" },
];
const modelSchema = z.object({
  display_name: z.string().trim().min(1, "请填写名称"),
  vendor: z.string().trim().min(1, "请填写原厂"),
  kind: z.enum(["text", "image", "video", "audio", "embedding"]),
  description: z.string(),
});
const priceSchema = z.object({
  input: z.string(), output: z.string(), image_count: z.string(), video_second: z.string(), audio_second: z.string(),
  currency: z.string().min(1),
});
type ModelFields = z.infer<typeof modelSchema>;
type PriceFields = z.infer<typeof priceSchema>;

function mergedCapabilities(model: AdminModel | undefined, values: ModelFields): Record<string, unknown> {
  const caps = { ...(model?.capabilities ?? {}) };
  caps.kind = values.kind;
  caps.description = values.description.trim();
  return caps;
}

function pricePayload(values: PriceFields): Record<string, unknown> {
  const payload: Record<string, unknown> = { currency: values.currency };
  const sell = millionDim(values.input, values.output);
  if (sell) payload.customer_sell = sell;
  for (const key of ["image_count", "video_second", "audio_second"] as const) {
    if (values[key].trim()) payload[key] = values[key].trim();
  }
  return payload;
}

export default function AdminModelEditPage() {
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
      if (!res.ok) { setMessage(body.error?.message || "保存失败"); return false; }
      if (isNew && !body.item?.id) { setMessage("已创建模型，但未收到调用 ID，请返回列表查看"); return false; }
      setMessage(isNew ? "模型与首次售价已保存" : "已保存模型信息");
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
        priceForm.setError(key, { message: value ? "请输入非负数字" : "请填写售价" });
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
      if (!res.ok) { setLifeError(body.error?.message || "操作失败"); return false; }
      setLifeError("");
      await reload();
      return true;
    } catch { setLifeError(confirmNetworkUnavailable); return false; }
  }

  if (!isNew && (query.isPending || query.isError || query.data?.error || !model)) return <AdminShell>
      <h1 className="text-2xl font-semibold">{model?.display_name || publicId}</h1><section role={query.isPending ? "status" : "alert"}><p>{query.isPending ? "正在读取模型…" : "读取模型失败，未修改配置。"}</p>{!query.isPending ? <Button variant="outline" onClick={()=>void query.refetch()}>重试读取</Button>:null}</section></AdminShell>;
  return (
    <AdminShell>
      <h1 className="text-2xl font-semibold">{isNew ? "创建模型" : model?.display_name || publicId}</h1>
      <Link className="text-sm text-brand-emphasis hover:underline" href={safeReturnHref(returnTo, "/admin/models")}>返回模型列表</Link>
      {query.data?.error ? <p className="text-sm text-danger">{query.data.error.message}</p> : null}

      {!isNew && model ? <ModelServicePanel model={model} /> : null}
      <IfCan action="models.write">
        <section className="rounded-card border border-hairline bg-canvas-raised p-6">
          <h3 className="text-base font-semibold">模型信息</h3>
          <Form {...modelForm}>
            <form className="mt-4 grid max-w-2xl gap-4" onSubmit={(event) => event.preventDefault()}>
              {!isNew ? <p className="text-sm text-ink-secondary">调用 ID <code className="ml-2 font-mono text-ink-primary">{publicId}</code></p> : null}
              <TextField control={modelForm.control} name="display_name" label="名称" />
              <TextField control={modelForm.control} name="vendor" label="原厂" placeholder="例如 alibaba" />
              <AdminSelectField control={modelForm.control} name="kind" label="类型" options={kinds} />
              <TextField control={modelForm.control} name="description" label="简介" />
              {!isNew ? <ConfirmButton
                size="sm" title="确认保存模型"
                description="保存模型信息。公开模型标识创建后不可修改。"
                validate={() => modelForm.trigger()}
                onConfirm={confirmFormSubmit(modelForm.handleSubmit, saveModel)}
              >保存模型</ConfirmButton> : null}
              {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
            </form>
          </Form>
        </section>
      </IfCan>

      {(isNew || model) ? <>
        <IfCan action="prices.write">
          <section className="rounded-card border border-hairline bg-canvas-raised p-6">
            <h3 className="text-base font-semibold">售价</h3>
            <p className="mt-1 text-sm text-ink-secondary">{isNew ? "首次售价随模型一起保存，之后可单独改价。" : "改价只影响之后的请求。"}</p>
            <Form {...priceForm}>
              <form className="mt-4 grid max-w-xl gap-3" onSubmit={(event) => event.preventDefault()}>
                {kind === "text" || kind === "embedding" ? <TextField control={priceForm.control} name="input" label="输入售价" suffix="美元/M token" /> : null}
                {kind === "text" ? <TextField control={priceForm.control} name="output" label="输出售价" suffix="美元/M token" /> : null}
                {kind === "image" ? <TextField control={priceForm.control} name="image_count" label="每张图片售价" suffix="美元/张" /> : null}
                {kind === "video" ? <TextField control={priceForm.control} name="video_second" label="每秒视频售价" suffix="美元/秒" /> : null}
                {kind === "audio" ? <TextField control={priceForm.control} name="audio_second" label="每秒音频售价" suffix="美元/秒" /> : null}
                {(kind === "image" || kind === "video" || kind === "audio") && model?.sell_price?.media &&
                  !model?.sell_price?.[kind === "image" ? "image_count" : kind === "video" ? "video_second" : "audio_second"] ?
                  <p className="text-sm text-ink-secondary">旧价格 {String(model.sell_price.media)}／媒体单位，未标明当前计价单位。请确认后填写上方售价。</p> : null}
                <AdminSelectField control={priceForm.control} name="currency" label="币种" options={[{ value: "USD", label: "美元 USD" }]} />
                {isNew ? <ConfirmButton
                  size="sm" title="确认创建模型" description="模型信息和首次售价将一起保存；创建后可发布模型。"
                  error={message} validate={validateNewModel}
                  onConfirm={confirmFormSubmit(modelForm.handleSubmit, saveModel)}
                >创建模型</ConfirmButton> : <SealConfirm
                  size="sm" title="确认发布新价格" description="历史账单保持原价格快照。"
                  validate={() => priceForm.trigger()}
                  onConfirm={confirmFormSubmit(priceForm.handleSubmit, async (values) => {
                    try {
                      const payload = { ...pricePayload(values), model: publicId };
                      const res = await fetch(`${apiBase}/admin/price-books`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify(payload) });
                      const body = await res.json();
                      if (!res.ok) { setPriceMessage(body.error?.message || "发布价格失败"); return false; }
                      setPriceMessage(`已发布 ${publishedPriceLabel(body.price, publicId)}`);
                      await reload();
                      return true;
                    } catch (err) { setPriceMessage(err instanceof Error ? err.message : confirmNetworkUnavailable); return false; }
                  })}
                >发布价格</SealConfirm>}
                {priceMessage ? <p className="text-sm text-ink-secondary">{priceMessage}</p> : null}
              </form>
            </Form>
          </section>
        </IfCan>
        {!isNew && model ? <IfCan action="models.write">
          <section className="rounded-card border border-hairline bg-canvas-raised p-6">
            <div className="flex flex-wrap items-center gap-3">
              <h3 className="text-base font-semibold">模型状态</h3>
              <Badge tone={catalogStatusTone(model.status)}>{model.status === "published" && model.config_ready === false ? "待补配置" : modelStatusLabel(model.status)}</Badge>
            </div>
            <p className="mt-1 text-sm text-ink-secondary">模型配置完整后可在路由组选择；发布本身不会启用路由或授权渠道。</p>
            <div className="mt-4 flex flex-wrap gap-2">
              {model.status !== "published" ? <ConfirmButton
                size="sm" title="确认发布模型" description="仅发布模型配置，不创建路由和渠道授权。"
                error={lifeError} validate={() => {
                  if (modelForm.formState.isDirty || priceForm.formState.isDirty) {
                    setLifeError("请先保存模型信息或发布新价格");
                    return false;
                  }
                  setLifeError("");
                  return true;
                }} onConfirm={() => lifecycle("publish")}
              >发布模型</ConfirmButton> : <>
                <ConfirmButton size="sm" variant="outline" title="确认弃用模型" description="弃用后客户目录隐藏，历史账单保留。" error={lifeError} onConfirm={() => lifecycle("deprecate")}>弃用模型</ConfirmButton>
              </>}
            </div>
            {lifeError ? <p className="mt-3 text-sm text-danger">{lifeError}</p> : null}
          </section>
        </IfCan> : null}
        {!isNew && model ? <details className="rounded-card border border-hairline bg-canvas-raised p-6">
          <summary className="cursor-pointer text-sm font-medium">查看价格版本</summary>
          <div className="mt-4">
            <AdminListPanel<PriceBook>
              path={`/admin/price-books?q=${encodeURIComponent(publicId)}`}
              title="价格版本" columns={priceBookColumns} emptyTitle="还没有价格版本" emptyDetail="发布售价后会记录版本。"
            />
          </div>
        </details> : null}
      </> : null}
    </AdminShell>
  );
}
