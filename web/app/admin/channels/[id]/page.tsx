"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { AdminShell } from "../../shell";
import { AdminListPanel } from "../../list-panel";
import { ChannelQuotaPanel } from "../quota-panel";
import { ChannelModelsPanel } from "../models-panel";
import { ChannelPaymentReadiness } from "../payment-readiness";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { CHANNEL_TYPES, STATUS_OPTIONS, channelTypeLabel, partnerHref } from "@/lib/tenants";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string; parent_id?: string };
type Role = { id: string; channel_org_id: string; type: string; parent_id?: string; status: string };
type ItemResponse = { item?: Channel; error?: { message?: string } };

const patchSchema = z.object({
  status: z.string().trim().min(1, "请选择状态"),
  type: z.string().trim().min(1, "请选择类型"),
  brand_id: z.string().trim(),
});

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

export default function AdminChannelDetailPage() {
  const params = useParams<{ id: string }>();
  const raw = params.id;
  const id = decodeURIComponent(Array.isArray(raw) ? raw[0] : raw || "");
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("停用后冻结新消费（聊天/媒体 403），余额和历史仍保留。不要停用 chn_official_a / chn_reseller_b / chn_oem_c。");
  const query = useQuery({
    queryKey: ["/admin/channels", id],
    queryFn: () => apiClient<ItemResponse>("GET", `/admin/channels/${id}`),
  });
  const item = query.data?.item;
  const form = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    values: {
      status: item?.status || "active",
      type: item?.type || "B",
      brand_id: item?.brand_id || "",
    },
  });

  return (
    <AdminShell>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link href="/admin/channels" className="text-sm text-brand-emphasis no-underline hover:underline">
            返回列表
          </Link>
          <h2 className="mt-3 text-lg font-semibold tracking-tight">渠道详情</h2>
          <p className="mt-1 text-sm text-ink-secondary">{item ? `${item.code} · ${channelTypeLabel(item.type)}` : id}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {editing ? (
            <>
              <Button size="sm" variant="outline" onClick={() => setEditing(false)}>
                取消
              </Button>
              <ConfirmButton
                size="sm"
                title="确认保存渠道"
                description="停用后冻结新消费。不要停用 chn_official_a / chn_reseller_b / chn_oem_c。"
                validate={() => form.trigger()}
                onConfirm={form.handleSubmit(async (values) => {
                  const res = await fetch(`${apiBase}/admin/channels/${id}`, {
                    method: "PATCH",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: JSON.stringify(values),
                  });
                  const body = await res.json();
                  if (!res.ok) {
                    setMessage(body.error?.message || "保存失败");
                    return;
                  }
                  setMessage(`已保存 ${body.item?.id} → ${body.item?.status} / ${body.item?.type}`);
                  setEditing(false);
                  await queryClient.invalidateQueries({ queryKey: ["/admin/channels", id] });
                })}
              >
                保存渠道
              </ConfirmButton>
            </>
          ) : (
            <Button size="sm" onClick={() => setEditing(true)}>
              编辑
            </Button>
          )}
        </div>
      </div>
      {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}
      <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
        <h3 className="mb-3 text-lg font-medium">租户字段</h3>
        {editing ? (
          <Form {...form}>
            <form className="grid max-w-xl gap-3" onSubmit={(event) => event.preventDefault()}>
              <p className="text-sm text-ink-secondary">改状态、类型或品牌。code 创建后不可改。</p>
              <FormField
                control={form.control}
                name="type"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>租户类型</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label="租户类型" {...field}>
                        {CHANNEL_TYPES.map((option) => (
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
              <FormField
                control={form.control}
                name="status"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>状态</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label="渠道状态" {...field}>
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
              <FormField
                control={form.control}
                name="brand_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>品牌 ID</FormLabel>
                    <FormControl>
                      <input className={selectClass} aria-label="品牌 ID" {...field} />
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
              <dt className="text-ink-secondary">ID</dt>
              <dd className="font-mono">{item?.id || id}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">Code</dt>
              <dd>{item?.code || "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">租户类型</dt>
              <dd>{item ? channelTypeLabel(item.type) : "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">状态</dt>
              <dd>{item?.status || "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">品牌</dt>
              <dd>{item?.brand_id || "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 py-2">
              <dt className="text-ink-secondary">上级渠道</dt>
              <dd>{item?.parent_id || "无"}</dd>
            </div>
          </dl>
        )}
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      <ChannelModelsPanel channelID={id} />
      <ChannelQuotaPanel channelID={id} channelType={item?.type || ""} />
      <ChannelPaymentReadiness channelID={id} />
      <AdminListPanel<Role>
        path={`/admin/acquisition-roles?channel_id=${encodeURIComponent(id)}&type=agent`}
        title="本租户代理商"
        rowHref={(row) => partnerHref(String(row.id))}
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "type", header: "角色" },
          { accessorKey: "status", header: "状态" },
        ]}
      />
      <AdminListPanel<Role>
        path={`/admin/acquisition-roles?channel_id=${encodeURIComponent(id)}&type=kol`}
        title="本租户 KOL"
        rowHref={(row) => partnerHref(String(row.id))}
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "type", header: "层级" },
          { accessorKey: "parent_id", header: "上级" },
          { accessorKey: "status", header: "状态" },
        ]}
      />
    </AdminShell>
  );
}
