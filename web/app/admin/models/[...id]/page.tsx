"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ProviderSlugCombobox } from "@/components/provider-slug-combobox";
import { VendorCombobox } from "@/components/vendor-combobox";
import { CheckPills } from "@/components/check-pills";
import { ConfirmButton } from "@/components/confirm-button";
import { SealConfirm } from "@/components/seal-confirm";
import { TextField } from "@/components/text-field";
import { TokenizerCombobox } from "@/components/tokenizer-combobox";
import { AdminSelectField } from "@/components/admin-select-field";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Badge } from "@/components/ui/badge";
import { AdminListPanel } from "../../list-panel";
import { AdminShell } from "../../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";
import { priceBookColumns, publishedPriceLabel, type PriceBook } from "@/lib/price-book";
import {
  type AdminModel,
  capabilitiesToForm,
  formToCapabilities,
  formatSellPrice,
  KNOWN_ENDPOINTS,
  KNOWN_MODALITIES,
  KNOWN_PARAMETERS,
  optionUnion,
  vendorLabel,
} from "@/lib/catalog";
import { catalogStatusTone, formatProviderSlugs, modelLifecycleEnabled, modelStatusLabel, syncStateLabel } from "@/lib/catalog-admin";
import { CATALOG_LABEL } from "@/lib/catalog-copy";
import { IfCan } from "@/components/rbac/if-can";

const attrSchema = z.object({
  display_name: z.string().trim().min(1, "请填写显示名"),
  vendor: z.string().trim().min(1, "请选择原厂"),
  supported_parameters: z.array(z.string()),
  input_modalities: z.array(z.string()),
  output_modalities: z.array(z.string()),
  supported_endpoints: z.array(z.string()),
  tokenizer: z.string(),
  rest_json: z.string(),
});

const priceSchema = z.object({
  input: z.string(),
  output: z.string(),
  wholesale_input: z.string(),
  wholesale_output: z.string(),
  upstream_cost_input: z.string(),
  upstream_cost_output: z.string(),
  channel_input: z.string(),
  channel_output: z.string(),
  video_second: z.string(),
  image_count: z.string(),
  audio_second: z.string(),
  currency: z.string().trim().min(1, "请填写币种"),
});

const attachSchema = z.object({
  provider_id: z.string().trim().min(1, "请选择提供商"),
  upstream_model_id: z.string().trim().min(1, "请填写上游模型标识"),
});

export default function AdminModelEditPage() {
  const params = useParams<{ id?: string | string[] }>();
  const publicId = useMemo(() => {
    const raw = params.id;
    if (Array.isArray(raw)) {
      return raw.join("/");
    }
    return raw || "";
  }, [params.id]);
  const queryClient = useQueryClient();
  const [attrMessage, setAttrMessage] = useState("只改客户看到的名字和能力，不改公开模型标识。请勿修改 tokenhub/echo-1。");
  const [priceMessage, setPriceMessage] = useState("新价格只影响之后的请求，旧账单保持快照。");
  const [lifeMessage, setLifeMessage] = useState("草稿需由另一位管理员审核后再单独发布。创建人不能审核或发布。弃用不删除历史映射和价格。");
  const [attachMessage, setAttachMessage] = useState("把这个公开模型接到一家提供商，并填写该提供商内部的上游模型标识。");
  const query = useQuery({
    queryKey: ["/admin/models", publicId],
    queryFn: () => apiClient<{ item?: AdminModel; error?: { message?: string } }>("GET", `/admin/models/${publicId}`),
    enabled: publicId.length > 0,
  });
  const providersQuery = useQuery({
    queryKey: ["/admin/providers"],
    queryFn: () => apiClient<{ items?: { id: string; name: string; slug: string }[] }>("GET", "/admin/providers"),
  });
  const providerOptions = providersQuery.data?.items ?? [];
  const model = query.data?.item;
  const life = modelLifecycleEnabled(model?.status, model?.sync_state);
  const attrForm = useForm<z.infer<typeof attrSchema>>({
    resolver: zodResolver(attrSchema),
    defaultValues: {
      display_name: "",
      vendor: "",
      supported_parameters: [],
      input_modalities: [],
      output_modalities: [],
      supported_endpoints: [],
      tokenizer: "",
      rest_json: "",
    },
  });
  const selectedParams = attrForm.watch("supported_parameters");
  const selectedInputs = attrForm.watch("input_modalities");
  const selectedOutputs = attrForm.watch("output_modalities");
  const selectedEndpoints = attrForm.watch("supported_endpoints");
  const priceForm = useForm<z.infer<typeof priceSchema>>({
    resolver: zodResolver(priceSchema),
    defaultValues: {
      input: "",
      output: "",
      wholesale_input: "",
      wholesale_output: "",
      upstream_cost_input: "",
      upstream_cost_output: "",
      channel_input: "",
      channel_output: "",
      video_second: "",
      image_count: "",
      audio_second: "",
      currency: "USD",
    },
  });
  const attachForm = useForm<z.infer<typeof attachSchema>>({
    resolver: zodResolver(attachSchema),
    defaultValues: { provider_id: "", upstream_model_id: "" },
  });

  useEffect(() => {
    if (!model) {
      return;
    }
    attrForm.reset({
      display_name: model.display_name || "",
      vendor: model.vendor || "",
      ...capabilitiesToForm(model.capabilities),
    });
    priceForm.reset({
      input: perTokenToPerMillion(String(model.sell_price?.input ?? "")),
      output: perTokenToPerMillion(String(model.sell_price?.output ?? "")),
      wholesale_input: "",
      wholesale_output: "",
      upstream_cost_input: "",
      upstream_cost_output: "",
      channel_input: "",
      channel_output: "",
      video_second: String(model.sell_price?.video_second ?? ""),
      image_count: String(model.sell_price?.image_count ?? ""),
      audio_second: String(model.sell_price?.audio_second ?? ""),
      currency: String(model.sell_price?.currency ?? "USD"),
    });
  }, [model, attrForm, priceForm]);

  async function reload() {
    await queryClient.invalidateQueries({ queryKey: ["/admin/models", publicId] });
    await queryClient.invalidateQueries();
  }

  return (
    <AdminShell>
      <p className="text-sm text-ink-secondary">
        <Link className="text-brand-emphasis underline-offset-4 hover:underline" href="/admin/models">
          返回模型列表
        </Link>
      </p>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="th-eyebrow text-ink-mute">MODEL</p>
        <h2 className="mt-2 text-lg font-semibold tracking-tight">{model?.display_name || publicId || "模型详情"}</h2>
        <p className="mt-1 font-mono text-sm text-ink-secondary">{publicId || "缺少公开模型标识"}</p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Badge tone={catalogStatusTone(model?.status)}>{modelStatusLabel(model?.status)}</Badge>
          {model?.sync_state ? <Badge tone="neutral">{syncStateLabel(model.sync_state)}</Badge> : null}
          {model?.vendor ? (
            <span className="text-sm text-ink-secondary">
              {CATALOG_LABEL.vendor} {vendorLabel(model.vendor)}
            </span>
          ) : null}
          <span className="text-sm text-ink-secondary">
            {CATALOG_LABEL.providerPool} {model ? formatProviderSlugs(model.providers) : "需要平台管理员登录后才能加载。"}
          </span>
          {model ? <span className="text-sm text-ink-secondary">{formatSellPrice(model.sell_price)}</span> : null}
        </div>
        {query.data?.error ? <p className="mt-3 text-sm text-ink-secondary">{query.data.error.message}</p> : null}
      </section>
      <IfCan action="models.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">客户怎么看到它</h2>
        <p className="mt-1 text-sm text-ink-secondary">
          改显示名、原厂和能力。{CATALOG_LABEL.publicModelId}创建后不能改。
        </p>
        <Form {...attrForm}>
          <form className="mt-4 grid max-w-3xl gap-4" onSubmit={(event) => event.preventDefault()}>
            <TextField control={attrForm.control} name="display_name" label={CATALOG_LABEL.displayName} />
            <VendorCombobox control={attrForm.control} name="vendor" extra={[model?.vendor || ""]} />
            <CheckPills
              control={attrForm.control}
              name="supported_parameters"
              label="支持参数"
              hint="客户请求里能带的选项。点选即可，不必手打。"
              options={optionUnion(KNOWN_PARAMETERS, selectedParams)}
            />
            <CheckPills
              control={attrForm.control}
              name="input_modalities"
              label="能接收什么"
              hint="客户能送进模型的内容类型。"
              options={optionUnion(KNOWN_MODALITIES, selectedInputs)}
            />
            <CheckPills
              control={attrForm.control}
              name="output_modalities"
              label="能返回什么"
              hint="模型能产出的内容类型。"
              options={optionUnion(KNOWN_MODALITIES, selectedOutputs)}
            />
            <CheckPills
              control={attrForm.control}
              name="supported_endpoints"
              label="支持的调用方式"
              hint="客户走哪类接口。不确定就保持现状。"
              options={optionUnion(KNOWN_ENDPOINTS, selectedEndpoints)}
            />
            <TokenizerCombobox control={attrForm.control} name="tokenizer" />
            <details className="rounded-control border border-hairline bg-canvas p-4">
              <summary className="cursor-pointer text-sm font-medium">高级：其余能力字段</summary>
              <FormField
                control={attrForm.control}
                name="rest_json"
                render={({ field }) => (
                  <FormItem className="mt-3">
                    <FormLabel>其余能力字段</FormLabel>
                    <p className="text-sm text-ink-secondary">
                      只放上面勾选盖不住的键，例如 video_attributes。空白即可。
                    </p>
                    <FormControl>
                      <textarea
                        {...field}
                        rows={6}
                        spellCheck={false}
                        aria-label="其余能力字段"
                        placeholder="{}"
                        className="min-h-24 w-full rounded-control border border-hairline bg-canvas-raised px-3 py-2 font-mono text-sm leading-normal text-ink placeholder:text-ink-mute focus:border-brand-emphasis"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </details>
            <ConfirmButton
              size="sm"
              title="确认保存显示信息"
              description="只改显示名、原厂和能力，不改公开模型标识。请勿修改 tokenhub/echo-1。"
              validate={() => attrForm.trigger()}
              onConfirm={attrForm.handleSubmit(async (values) => {
                let capabilities: Record<string, unknown>;
                try {
                  capabilities = formToCapabilities(values);
                } catch {
                  setAttrMessage("其余能力字段必须是 JSON 对象");
                  return;
                }
                const res = await fetch(`${apiBase}/admin/models/${publicId}`, {
                  method: "PATCH",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    display_name: values.display_name,
                    vendor: values.vendor,
                    capabilities,
                  }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setAttrMessage(body.error?.message || "保存失败");
                  return;
                }
                setAttrMessage(`已保存 ${body.item?.id} → ${body.item?.display_name}`);
                await reload();
              })}
            >
              保存显示信息
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{attrMessage}</p>
          </form>
        </Form>
      </section>
      </IfCan>
      <IfCan action="prices.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">定价</h2>
        <p className="mt-1 text-sm text-ink-secondary">
          发布新价格只影响之后的请求，旧账单保持快照。空着的格子不会覆盖已有单价。Token 价按每百万 token 的美元填写，例如 2 表示 $2/M。
        </p>
        <Form {...priceForm}>
          <form className="mt-4 grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <div className="grid gap-2 sm:grid-cols-2">
              <TextField control={priceForm.control} name="upstream_cost_input" label="成本 输入" suffix="美元/M" />
              <TextField control={priceForm.control} name="upstream_cost_output" label="成本 输出" suffix="美元/M" />
              <TextField control={priceForm.control} name="wholesale_input" label="批发 输入" suffix="美元/M" />
              <TextField control={priceForm.control} name="wholesale_output" label="批发 输出" suffix="美元/M" />
              <TextField control={priceForm.control} name="input" label="售价 输入" suffix="美元/M" />
              <TextField control={priceForm.control} name="output" label="售价 输出" suffix="美元/M" />
              <TextField control={priceForm.control} name="channel_input" label="渠道覆盖 输入" suffix="美元/M" />
              <TextField control={priceForm.control} name="channel_output" label="渠道覆盖 输出" suffix="美元/M" />
            </div>
            <TextField control={priceForm.control} name="video_second" label="视频秒单价" suffix="美元/秒" />
            <TextField control={priceForm.control} name="image_count" label="图片次单价" suffix="美元/张" />
            <TextField control={priceForm.control} name="audio_second" label="音频秒单价" suffix="美元/秒" />
            <AdminSelectField
              control={priceForm.control}
              name="currency"
              label="币种"
              options={[
                { value: "USD", label: "美元 USD" },
                { value: "CNY", label: "人民币 CNY" },
              ]}
            />
            <SealConfirm
              size="sm"
              title="新牌价只约束之后的请求，已入账金额不会改写。"
              description="当前 published 会标成 superseded。历史版本保持只读快照。"
              validate={() => priceForm.trigger()}
              onConfirm={priceForm.handleSubmit(async (values) => {
                const payload: Record<string, unknown> = { model: publicId, currency: values.currency };
                let sell: Record<string, string> | undefined;
                let wholesale: Record<string, string> | undefined;
                let upstream: Record<string, string> | undefined;
                let channel: Record<string, string> | undefined;
                try {
                  sell = millionDim(values.input, values.output);
                  wholesale = millionDim(values.wholesale_input, values.wholesale_output);
                  upstream = millionDim(values.upstream_cost_input, values.upstream_cost_output);
                  channel = millionDim(values.channel_input, values.channel_output);
                } catch (err) {
                  setPriceMessage(err instanceof Error ? err.message : "单价无效");
                  return;
                }
                if (sell) {
                  payload.customer_sell = sell;
                }
                if (wholesale) {
                  payload.wholesale = wholesale;
                }
                if (upstream) {
                  payload.upstream_cost = upstream;
                }
                if (channel) {
                  payload.channel_override = channel;
                }
                for (const key of ["video_second", "image_count", "audio_second"] as const) {
                  if (values[key].trim()) {
                    payload[key] = values[key].trim();
                  }
                }
                const res = await fetch(`${apiBase}/admin/price-books`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify(payload),
                });
                const body = await res.json();
                setPriceMessage(res.ok ? `已发布 ${publishedPriceLabel(body.price, publicId)}` : body.error?.message || "发布失败");
                if (res.ok) {
                  await reload();
                }
              })}
            >
              发布价格
            </SealConfirm>
            <p className="text-sm text-ink-secondary">{priceMessage}</p>
          </form>
        </Form>
      </section>
      </IfCan>
      <IfCan action="models.attach">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">接到哪家提供商</h2>
        <p className="mt-1 text-sm text-ink-secondary">
          {CATALOG_LABEL.publicModelId}已锁定为 <span className="font-mono">{publicId || "缺少标识"}</span>。
          {CATALOG_LABEL.upstreamModelId}可以不同。
        </p>
        <Form {...attachForm}>
          <form className="mt-4 grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <ProviderSlugCombobox
              control={attachForm.control}
              name="provider_id"
              label={CATALOG_LABEL.provider}
              options={providerOptions}
              placeholder="输入名称或标识筛选"
            />
            <TextField control={attachForm.control} name="upstream_model_id" label={CATALOG_LABEL.upstreamModelId} placeholder="这家提供商内部的模型名" />
            <ConfirmButton
              size="sm"
              title="确认接到提供商"
              description={`${CATALOG_LABEL.upstreamModelId}可以与${CATALOG_LABEL.publicModelId}不同。`}
              validate={() => attachForm.trigger()}
              onConfirm={attachForm.handleSubmit(async (values) => {
                const res = await fetch(`${apiBase}/admin/models/attach`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    public_id: publicId,
                    provider_id: values.provider_id,
                    upstream_model_id: values.upstream_model_id,
                  }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setAttachMessage(body.error?.message || "关联失败");
                  return;
                }
                attachForm.reset({ provider_id: "", upstream_model_id: "" });
                setAttachMessage(`已关联 ${publicId} → ${values.provider_id}`);
                await reload();
              })}
            >
              接到提供商
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{attachMessage}</p>
          </form>
        </Form>
      </section>
      </IfCan>
      <IfCan action="models.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">上架</h2>
        <p className="mt-1 text-sm text-ink-secondary">
          当前 {modelStatusLabel(model?.status)}
          {model?.sync_state ? ` · ${syncStateLabel(model.sync_state)}` : ""}
          。请勿修改 tokenhub/echo-1。已上架的模型不能再审核或重复发布。
        </p>
        <div className="mt-4 flex flex-wrap gap-2">
          <ConfirmButton
            size="sm"
            disabled={!life.approve}
            title="确认通过模型"
            description="只标记审核通过，不会发布到客户目录。创建人不能审核自己建的模型。"
            onConfirm={async () => {
              const res = await fetch(`${apiBase}/admin/models/review`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ public_id: publicId, action: "approve" }),
              });
              const body = await res.json();
              setLifeMessage(res.ok ? `已通过 ${body.item?.id} → ${body.item?.sync_state}` : body.error?.message || "审核失败");
              await reload();
            }}
          >
            通过
          </ConfirmButton>
          <ConfirmButton
            size="sm"
            variant="outline"
            disabled={!life.reject}
            title="确认拒绝模型"
            description="拒绝后不能发布，需要重新通过。"
            onConfirm={async () => {
              const res = await fetch(`${apiBase}/admin/models/review`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ public_id: publicId, action: "reject" }),
              });
              const body = await res.json();
              setLifeMessage(res.ok ? `已拒绝 ${body.item?.id} → ${body.item?.sync_state}` : body.error?.message || "拒绝失败");
              await reload();
            }}
          >
            拒绝
          </ConfirmButton>
          <ConfirmButton
            size="sm"
            disabled={!life.publish}
            title="确认发布模型"
            description="必须先审核通过。创建人不能发布自己建的模型。"
            onConfirm={async () => {
              const res = await fetch(`${apiBase}/admin/models/publish`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ public_id: publicId }),
              });
              const body = await res.json();
              setLifeMessage(res.ok ? `已发布 ${body.item?.id} → ${body.item?.status}` : body.error?.message || "发布失败");
              await reload();
            }}
          >
            发布
          </ConfirmButton>
          <ConfirmButton
            size="sm"
            variant="outline"
            disabled={!life.deprecate}
            title="确认弃用模型"
            description="只改状态，不删除历史映射和价格版本。"
            onConfirm={async () => {
              const res = await fetch(`${apiBase}/admin/models/deprecate`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ public_id: publicId }),
              });
              const body = await res.json();
              setLifeMessage(res.ok ? `已弃用 ${body.item?.id} → ${body.item?.status}` : body.error?.message || "弃用失败");
              await reload();
            }}
          >
            弃用此模型
          </ConfirmButton>
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{lifeMessage}</p>
      </section>
      </IfCan>
      {publicId ? (
        <AdminListPanel<PriceBook>
          path={`/admin/price-books?q=${encodeURIComponent(publicId)}`}
          title="本模型价格版本"
          columns={priceBookColumns}
          emptyTitle="还没有价格版本"
          emptyDetail="发布后会出现版本号、生效时间和四列单价。"
        />
      ) : null}
    </AdminShell>
  );
}
