"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";

type Plan = {
  id: string;
  name: string;
  owner_type: string;
  owner_id: string;
  price_minor: number;
  status: string;
  review_reason?: string;
};

type ListResponse = { items?: Plan[]; error?: { message?: string } };

const createSchema = z.object({
  name: z.string().trim().min(1, "请填写套餐名"),
  owner_type: z.string().trim().min(1, "请填写归属"),
  price_minor: z.string().trim().min(1, "请填写价格"),
  unit_type: z.string().trim().min(1, "请填写权益单位"),
  included_amount: z.string().trim().min(1, "请填写权益数量"),
});

const archiveSchema = z.object({
  plan_id: z.string().trim().min(1, "请填写套餐 ID"),
});

const forceEndSchema = z.object({
  subscription_id: z.string().trim().min(1, "请填写订阅 ID"),
});

export default function AdminPlansPage() {
  const [status, setStatus] = useState("pending_review");
  const [reason, setReason] = useState("promo");
  const [message, setMessage] = useState("渠道低价或高风险媒体配额会进入待审核。通过或拒绝都会写审计。");
  const [writeMessage, setWriteMessage] = useState("平台套餐满 1 USD 会直接发布。不要下架 pln_echo_month，那是公共站演示套餐。");
  const [renewMessage, setRenewMessage] = useState("强制到期和续费扫描只在沙箱可用。不要对还在演示的订阅乱拨时钟。");
  const queryClient = useQueryClient();
  const path = status ? `/admin/plans?status=${encodeURIComponent(status)}` : "/admin/plans";
  const query = useQuery({
    queryKey: [path],
    queryFn: () => apiClient<ListResponse>("GET", path),
  });
  const items = query.data?.items ?? [];
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { name: "", owner_type: "platform", price_minor: "1000000", unit_type: "usd_credit", included_amount: "1000000" },
  });
  const archiveForm = useForm<z.infer<typeof archiveSchema>>({
    resolver: zodResolver(archiveSchema),
    defaultValues: { plan_id: "" },
  });
  const forceEndForm = useForm<z.infer<typeof forceEndSchema>>({
    resolver: zodResolver(forceEndSchema),
    defaultValues: { subscription_id: "" },
  });

  async function review(id: string, action: "approve" | "reject") {
    const res = await fetch(`${apiBase}/admin/plans/${id}/review`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ action, reason }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已${action === "approve" ? "通过" : "拒绝"} ${body.item?.id}` : body.error?.message || "审核失败");
    await queryClient.invalidateQueries({ queryKey: [path] });
  }

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="planReview" className="mb-3 text-xl font-medium" />
        <p className="mb-3 text-sm text-ink-secondary">低于 1 USD、超额权益或高风险视频秒数的渠道套餐会停在 pending_review。</p>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Button size="sm" variant={status === "pending_review" ? "default" : "outline"} onClick={() => setStatus("pending_review")}>
            待审核
          </Button>
          <Button size="sm" variant={status === "" ? "default" : "outline"} onClick={() => setStatus("")}>
            全部套餐
          </Button>
          <Input className="w-48" value={reason} onChange={(e) => setReason(e.target.value)} aria-label="审核原因" placeholder="审核原因" />
        </div>
        {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="px-2 py-2">名称</th>
              <th className="px-2 py-2">归属</th>
              <th className="px-2 py-2">价格</th>
              <th className="px-2 py-2">状态</th>
              <th className="px-2 py-2">原因</th>
              <th className="px-2 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-hairline/80">
                <td className="px-2 py-2 text-ink">{item.name}</td>
                <td className="px-2 py-2 text-ink-secondary">
                  {item.owner_type} / {item.owner_id}
                </td>
                <td className="px-2 py-2 text-ink-secondary">{item.price_minor}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.status}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.review_reason || "-"}</td>
                <td className="px-2 py-2">
                  {item.status === "pending_review" ? (
                    <div className="flex flex-wrap gap-2">
                      <ConfirmButton size="sm" title="确认通过套餐" description={`将通过 ${item.name}，并写入审计。`} onConfirm={() => review(item.id, "approve")}>
                        通过
                      </ConfirmButton>
                      <ConfirmButton size="sm" variant="outline" title="确认拒绝套餐" description={`将拒绝 ${item.name}，并写入审计。`} onConfirm={() => review(item.id, "reject")}>
                        拒绝
                      </ConfirmButton>
                    </div>
                  ) : (
                    item.id
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      <Form {...createForm}>
        <form className="rounded-card border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="createPlan" className="mb-3 text-xl font-medium" />
          <p className="mb-3 text-sm text-ink-secondary">价格单位是 micro-USD。渠道套餐低于 1 USD 会进 pending_review；平台套餐会直接 published。</p>
          <div className="mb-3 grid max-w-xl gap-2">
            <TextField control={createForm.control} name="name" label="创建用套餐名" />
            <TextField control={createForm.control} name="owner_type" label="创建用归属" placeholder="创建用归属 platform" />
            <TextField control={createForm.control} name="price_minor" label="创建用价格" placeholder="创建用价格 1000000" />
            <TextField control={createForm.control} name="unit_type" label="创建用权益单位" placeholder="创建用权益单位 usd_credit" />
            <TextField control={createForm.control} name="included_amount" label="创建用权益数量" />
          </div>
          <ConfirmButton
            size="sm"
            title="确认创建套餐"
            description="平台套餐满 1 USD 会直接发布。"
            validate={() => createForm.trigger()}
            onConfirm={createForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/plans`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({
                  name: values.name,
                  owner_type: values.owner_type || "platform",
                  price_minor: Number(values.price_minor || 0),
                  items: [
                    {
                      unit_type: values.unit_type || "usd_credit",
                      included_amount: Number(values.included_amount || 0),
                    },
                  ],
                }),
              });
              const body = await res.json();
              if (!res.ok) {
                setWriteMessage(body.error?.message || "创建失败");
                return;
              }
              createForm.reset();
              setWriteMessage(`已创建 ${body.item?.id} ${body.item?.name} → ${body.item?.status}`);
              await queryClient.invalidateQueries();
            })}
          >
            创建套餐
          </ConfirmButton>
        </form>
      </Form>
      <Form {...archiveForm}>
        <form className="rounded-card border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="archivePlan" className="mb-3 text-xl font-medium" />
          <p className="mb-3 text-sm text-ink-secondary">只改成 archived，不删历史订阅。不要下架 pln_echo_month。</p>
          <div className="mb-3 grid max-w-xl gap-2">
            <TextField control={archiveForm.control} name="plan_id" label="下架用套餐 ID" />
          </div>
          <ConfirmButton
            size="sm"
            title="确认下架套餐"
            description="不要下架 pln_echo_month。下架后历史订阅仍保留。"
            validate={() => archiveForm.trigger()}
            onConfirm={archiveForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/plans/${values.plan_id}`, {
                method: "PATCH",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ status: "archived" }),
              });
              const body = await res.json();
              if (!res.ok) {
                setWriteMessage(body.error?.message || "下架失败");
                return;
              }
              setWriteMessage(`已下架 ${body.item?.id} → ${body.item?.status}`);
              await queryClient.invalidateQueries();
            })}
          >
            下架套餐
          </ConfirmButton>
        </form>
      </Form>
      <p className="text-sm text-ink-secondary">{writeMessage}</p>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="renewScan" className="mb-3 text-xl font-medium" />
        <p className="mb-3 text-sm text-ink-secondary">
          强制到期把 period_end 拨到过去，再扫描才会走重试/宽限期。生产默认禁止。不强制确认头。
        </p>
        <Form {...forceEndForm}>
          <form
            className="mb-3 flex flex-wrap items-end gap-2"
            onSubmit={forceEndForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/subscriptions/${values.subscription_id}/force-period-end`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: "{}",
              });
              const body = await res.json();
              setRenewMessage(res.ok ? `已拨时钟 ${values.subscription_id}` : body.error?.message || "拨时钟失败");
            })}
          >
            <TextField control={forceEndForm.control} name="subscription_id" label="强制到期用订阅 ID" showLabel={false} className="w-72" />
            <Button size="sm" type="submit" variant="outline">
              强制到期
            </Button>
          </form>
        </Form>
        <Button
          size="sm"
          onClick={async () => {
            const res = await fetch(`${apiBase}/admin/subscriptions/process-renewals`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: "{}",
            });
            const body = await res.json();
            setRenewMessage(res.ok ? `续费扫描 processed=${body.processed ?? 0}` : body.error?.message || "扫描失败");
          }}
        >
          续费扫描
        </Button>
        <p className="mt-3 text-sm text-ink-secondary">{renewMessage}</p>
      </section>
    </AdminShell>
  );
}
