"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { SealConfirm } from "@/components/seal-confirm";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { AdminListPanel } from "../../list-panel";
import { AdminShell } from "../../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { priceBookColumns, publishedPriceLabel, type PriceBook } from "@/lib/price-book";
import {
  type AdminModel,
  buildCapabilities,
  extraCapabilitiesJSON,
  formatSellPrice,
  supportedParametersText,
} from "@/lib/catalog";
import { formatProviderSlugs, syncStateLabel } from "@/lib/catalog-admin";
import { IfCan } from "@/components/rbac/if-can";

const attrSchema = z.object({
  display_name: z.string().trim().min(1, "请填写显示名"),
  vendor: z.string().trim().min(1, "请填写厂商"),
  supported_parameters: z.string(),
  capabilities_json: z.string(),
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

function dim(input: string, output: string) {
  const next: Record<string, string> = {};
  if (input.trim()) {
    next.input = input.trim();
  }
  if (output.trim()) {
    next.output = output.trim();
  }
  return Object.keys(next).length > 0 ? next : undefined;
}

const attachSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写提供商 ID"),
  upstream_model_id: z.string().trim().min(1, "请填写上游模型名"),
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
  const [attrMessage, setAttrMessage] = useState("属性与定价写入目录服务，非前端本地数据。请勿修改 tokenhub/echo-1。");
  const [priceMessage, setPriceMessage] = useState("新价格只影响之后的请求，旧账单保持快照。");
  const [lifeMessage, setLifeMessage] = useState("draft 需由另一位管理员审核后再单独发布。创建人不能审核或发布。弃用不删除历史映射和价格。");
  const [attachMessage, setAttachMessage] = useState("为当前公开模型增加一条上游接入途径。上游模型名可以与公开 ID 不同。");
  const query = useQuery({
    queryKey: ["/admin/models", publicId],
    queryFn: () => apiClient<{ item?: AdminModel; error?: { message?: string } }>("GET", `/admin/models/${publicId}`),
    enabled: publicId.length > 0,
  });
  const model = query.data?.item;
  const attrForm = useForm<z.infer<typeof attrSchema>>({
    resolver: zodResolver(attrSchema),
    defaultValues: { display_name: "", vendor: "", supported_parameters: "", capabilities_json: "" },
  });
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
      supported_parameters: supportedParametersText(model.capabilities),
      capabilities_json: extraCapabilitiesJSON(model.capabilities),
    });
    priceForm.reset({
      input: String(model.sell_price?.input ?? ""),
      output: String(model.sell_price?.output ?? ""),
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
      <IfCan action="models.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">编辑属性</h2>
        <p className="mt-1 text-sm text-ink-secondary">
          公开模型 <span className="font-mono">{publicId || "缺少 public id"}</span>
          {model?.vendor ? ` · 厂商 ${model.vendor}` : ""}
          {` · ${model?.status || query.data?.error?.message || "需要平台管理员登录后才能加载。"}`}
          {model?.sync_state ? ` · ${syncStateLabel(model.sync_state)}` : ""}
          {model ? ` · 途径 ${formatProviderSlugs(model.providers)}` : ""}
          {model ? ` · ${formatSellPrice(model.sell_price)}` : ""}
        </p>
        <Form {...attrForm}>
          <form className="mt-4 grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <TextField control={attrForm.control} name="display_name" label="显示名" />
            <TextField control={attrForm.control} name="vendor" label="厂商" />
            <TextField control={attrForm.control} name="supported_parameters" label="支持参数" placeholder="stream, tools, vision, json, reasoning" />
            <TextField control={attrForm.control} name="capabilities_json" label="额外 capabilities JSON" placeholder='{"output_modality":"video"}' />
            <ConfirmButton
              size="sm"
              title="确认保存属性"
              description="仅修改展示名、厂商和能力，不修改 public id。请勿修改 tokenhub/echo-1。"
              validate={() => attrForm.trigger()}
              onConfirm={attrForm.handleSubmit(async (values) => {
                let capabilities: Record<string, unknown>;
                try {
                  capabilities = buildCapabilities(values.supported_parameters, values.capabilities_json);
                } catch {
                  setAttrMessage("额外 capabilities 必须是 JSON 对象");
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
              保存属性
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{attrMessage}</p>
          </form>
        </Form>
      </section>
      </IfCan>
      <IfCan action="prices.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">定价</h2>
        <p className="mt-1 text-sm text-ink-secondary">发布新版本会把当前 published 标成 superseded。空字段不会覆盖已有维度。历史版本只读。</p>
        <Form {...priceForm}>
          <form className="mt-4 grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <div className="grid gap-2 sm:grid-cols-2">
              <TextField control={priceForm.control} name="upstream_cost_input" label="成本 输入" />
              <TextField control={priceForm.control} name="upstream_cost_output" label="成本 输出" />
              <TextField control={priceForm.control} name="wholesale_input" label="批发 输入" />
              <TextField control={priceForm.control} name="wholesale_output" label="批发 输出" />
              <TextField control={priceForm.control} name="input" label="售价 输入" />
              <TextField control={priceForm.control} name="output" label="售价 输出" />
              <TextField control={priceForm.control} name="channel_input" label="渠道覆盖 输入" />
              <TextField control={priceForm.control} name="channel_output" label="渠道覆盖 输出" />
            </div>
            <TextField control={priceForm.control} name="video_second" label="视频秒单价" />
            <TextField control={priceForm.control} name="image_count" label="图片次单价" />
            <TextField control={priceForm.control} name="audio_second" label="音频秒单价" />
            <TextField control={priceForm.control} name="currency" label="币种" />
            <SealConfirm
              size="sm"
              title="新牌价只约束之后的请求，已入账金额不会改写。"
              description="当前 published 会标成 superseded。历史版本保持只读快照。"
              validate={() => priceForm.trigger()}
              onConfirm={priceForm.handleSubmit(async (values) => {
                const payload: Record<string, unknown> = { model: publicId, currency: values.currency };
                const sell = dim(values.input, values.output);
                const wholesale = dim(values.wholesale_input, values.wholesale_output);
                const upstream = dim(values.upstream_cost_input, values.upstream_cost_output);
                const channel = dim(values.channel_input, values.channel_output);
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
        <h2 className="text-lg font-semibold tracking-tight">关联提供商</h2>
        <p className="mt-1 text-sm text-ink-secondary">
          公开 ID 已锁定为 <span className="font-mono">{publicId || "缺少 public id"}</span>。上游模型名可以与公开 ID 不同。
        </p>
        <Form {...attachForm}>
          <form className="mt-4 grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <TextField control={attachForm.control} name="provider_id" label="提供商 ID" placeholder="上游接入渠道，不是厂商名" />
            <TextField control={attachForm.control} name="upstream_model_id" label="上游模型名" placeholder="该提供商内部的模型 ID" />
            <ConfirmButton
              size="sm"
              title="确认关联提供商"
              description="上游名称可以与公开 ID 不同。"
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
              关联
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{attachMessage}</p>
          </form>
        </Form>
      </section>
      </IfCan>
      <IfCan action="models.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">上架</h2>
        <p className="mt-1 text-sm text-ink-secondary">当前状态 {model?.status || "未知"} · sync {model?.sync_state || "无"}。请勿修改 tokenhub/echo-1。</p>
        <div className="mt-4 flex flex-wrap gap-2">
          <ConfirmButton
            size="sm"
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
