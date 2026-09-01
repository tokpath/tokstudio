"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { AdminListPanel } from "../../list-panel";
import { AdminShell } from "../../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import {
  type AdminModel,
  buildCapabilities,
  extraCapabilitiesJSON,
  formatSellPrice,
  supportedParametersText,
} from "@/lib/catalog";

type PriceBook = { id: string; public_id: string; status: string };

const attrSchema = z.object({
  display_name: z.string().trim().min(1, "请填写显示名"),
  vendor: z.string().trim().min(1, "请填写厂商"),
  supported_parameters: z.string(),
  capabilities_json: z.string(),
});

const priceSchema = z.object({
  input: z.string(),
  output: z.string(),
  video_second: z.string(),
  image_count: z.string(),
  audio_second: z.string(),
  currency: z.string().trim().min(1, "请填写币种"),
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
  const [attrMessage, setAttrMessage] = useState("属性和定价都写到 catalog，不是前端 mock。不要改 tokenhub/echo-1。");
  const [priceMessage, setPriceMessage] = useState("新价格只影响之后的请求，旧账单保持快照。");
  const [lifeMessage, setLifeMessage] = useState("draft 要先换人审核，再单独发布。创建人不能审核或发布。弃用不删历史映射和价格。");
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
    defaultValues: { input: "", output: "", video_second: "", image_count: "", audio_second: "", currency: "USD" },
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
        <h2 className="text-xl font-medium">编辑属性</h2>
        <p className="mt-1 text-sm text-ink-secondary">
          {publicId || "缺少 public id"} · {model?.status || query.data?.error?.message || "需要平台管理员登录后才能加载。"}
          {model?.sync_state ? ` · sync ${model.sync_state}` : ""}
          {model?.providers?.length ? ` · providers ${model.providers.join(", ")}` : ""}
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
              description="只改展示名、厂商和能力，不改 public id。不要改 tokenhub/echo-1。"
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
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-xl font-medium">定价</h2>
        <p className="mt-1 text-sm text-ink-secondary">发布新版本会把当前 published 标成 superseded。空字段不会写入。</p>
        <Form {...priceForm}>
          <form className="mt-4 grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <TextField control={priceForm.control} name="input" label="输入单价" />
            <TextField control={priceForm.control} name="output" label="输出单价" />
            <TextField control={priceForm.control} name="video_second" label="视频秒单价" />
            <TextField control={priceForm.control} name="image_count" label="图片次单价" />
            <TextField control={priceForm.control} name="audio_second" label="音频秒单价" />
            <TextField control={priceForm.control} name="currency" label="币种" />
            <ConfirmButton
              size="sm"
              title="确认发布价格"
              description="新价格只影响之后的请求，旧账单保持快照。"
              validate={() => priceForm.trigger()}
              onConfirm={priceForm.handleSubmit(async (values) => {
                const payload: Record<string, string> = { model: publicId, currency: values.currency };
                for (const key of ["input", "output", "video_second", "image_count", "audio_second"] as const) {
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
                setPriceMessage(res.ok ? `已发布 ${body.price?.PublicID || publicId}` : body.error?.message || "发布失败");
                if (res.ok) {
                  await reload();
                }
              })}
            >
              发布价格
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{priceMessage}</p>
          </form>
        </Form>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <h2 className="text-xl font-medium">上架</h2>
        <p className="mt-1 text-sm text-ink-secondary">当前状态 {model?.status || "未知"} · sync {model?.sync_state || "无"}。不要改 tokenhub/echo-1。</p>
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
      {publicId ? (
        <AdminListPanel<PriceBook>
          path={`/admin/price-books?q=${encodeURIComponent(publicId)}`}
          title="本模型价格版本"
          columns={[
            { accessorKey: "public_id", header: "Model" },
            { accessorKey: "status", header: "Status" },
            { accessorKey: "id", header: "Version" },
          ]}
        />
      ) : null}
    </AdminShell>
  );
}
