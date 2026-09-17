"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { AdminShell } from "../../shell";
import { apiClient } from "@/lib/client";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { STATUS_OPTIONS, channelHref, isKOLType, roleTypeLabel } from "@/lib/tenants";
import { IfCan } from "@/components/rbac/if-can";

type Role = {
  id: string;
  channel_org_id: string;
  type: string;
  parent_id?: string;
  level?: number;
  status: string;
};
type ItemResponse = { item?: Role; error?: { message?: string } };

const patchSchema = z.object({
  status: z.string().trim().min(1, "请选择状态"),
});

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

export default function AdminPartnerDetailPage() {
  const params = useParams<{ id: string }>();
  const raw = params.id;
  const id = decodeURIComponent(Array.isArray(raw) ? raw[0] : raw || "");
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("代理商和 KOL 是租户内推广角色，不能自建提供商或模型。可改状态，层级在创建时固定。");
  const query = useQuery({
    queryKey: ["/admin/acquisition-roles", id],
    queryFn: () => apiClient<ItemResponse>("GET", `/admin/acquisition-roles/${id}`),
  });
  const item = query.data?.item;
  const form = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    values: { status: item?.status || "active" },
  });
  const title = item ? roleTypeLabel(item.type) : "推广角色";

  return (
    <AdminShell>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link href="/admin/channels" className="text-sm text-brand-emphasis no-underline hover:underline">
            返回列表
          </Link>
          <h2 className="mt-3 text-lg font-semibold tracking-tight">{isKOLType(item?.type || "") ? "KOL 详情" : "代理商详情"}</h2>
          <p className="mt-1 text-sm text-ink-secondary">
            {title} · {item?.id || id}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <IfCan action="partners.write">
          {editing ? (
            <>
              <Button size="sm" variant="outline" onClick={() => setEditing(false)}>
                取消
              </Button>
              <ConfirmButton
                size="sm"
                title="确认保存角色"
                description="只改状态。层级和所属租户创建后不可改。"
                validate={() => form.trigger()}
                onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                  try {
                  const res = await fetch(`${apiBase}/admin/acquisition-roles/${id}`, {
                    method: "PATCH",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: JSON.stringify(values),
                  });
                  const body = await res.json();
                  if (!res.ok) {
                    setMessage(body.error?.message || "保存失败");
                    return false;
                  }
                  setMessage(`已保存 ${body.item?.id} → ${body.item?.status}`);
                  setEditing(false);
                  await queryClient.invalidateQueries({ queryKey: ["/admin/acquisition-roles", id] });
                  return true;
                  } catch {
                    setMessage(confirmNetworkUnavailable);
                    return false;
                  }
})}
              >
                保存角色
              </ConfirmButton>
            </>
          ) : (
            <Button size="sm" onClick={() => setEditing(true)}>
              编辑
            </Button>
          )}
          </IfCan>
        </div>
      </div>
      {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}
      <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
        <h3 className="mb-3 text-lg font-medium">角色字段</h3>
        {editing ? (
          <Form {...form}>
            <form className="grid max-w-xl gap-3" onSubmit={(event) => event.preventDefault()}>
              <FormField
                control={form.control}
                name="status"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>状态</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label="角色状态" {...field}>
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
              <dt className="text-ink-secondary">ID</dt>
              <dd className="font-mono">{item?.id || id}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">角色类型</dt>
              <dd>{item ? roleTypeLabel(item.type) : "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">所属租户</dt>
              <dd>
                {item?.channel_org_id ? (
                  <Link href={channelHref(item.channel_org_id)} className="text-brand-emphasis no-underline hover:underline">
                    {item.channel_org_id}
                  </Link>
                ) : (
                  "—"
                )}
              </dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">上级</dt>
              <dd>
                {item?.parent_id ? (
                  <Link href={`/admin/partners/${item.parent_id}`} className="text-brand-emphasis no-underline hover:underline">
                    {item.parent_id}
                  </Link>
                ) : (
                  "无"
                )}
              </dd>
            </div>
            <div className="flex justify-between gap-4 border-b border-hairline py-2">
              <dt className="text-ink-secondary">层级</dt>
              <dd>{item?.level ?? "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 py-2">
              <dt className="text-ink-secondary">状态</dt>
              <dd>{item?.status || "—"}</dd>
            </div>
          </dl>
        )}
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
        <h3 className="mb-2 text-lg font-medium">模型资源</h3>
        <p className="text-sm text-ink-secondary">
          推广角色不拥有独立模型目录。可售模型继承所属渠道租户从平台获得的白名单，不能自己添加提供商和模型。
        </p>
      </section>
    </AdminShell>
  );
}
