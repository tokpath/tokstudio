"use client";

import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";

type Plan = { id?: string; name?: string; status?: string; owner_type?: string; owner_id?: string; price_minor?: number; review_reason?: string };

const schema = z.object({
  name: z.string().trim().min(1, "请填写渠道套餐名"),
  price_minor: z.string().trim().min(1, "请填写价格"),
  unit_type: z.string().trim().min(1, "请填写权益单位"),
  included_amount: z.string().trim().min(1, "请填写权益数量"),
});

export default function ChannelPlans() {
  const [items, setItems] = useState<Plan[]>([]);
  const [message, setMessage] = useState("渠道管理员可看本渠道套餐和平台套餐，看不到其他渠道的销售组合。");
  const [createMessage, setCreateMessage] = useState("低于 1 USD 或高风险配额会进 pending_review。归属会被后端写成当前渠道，改不了官方渠道。");
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", price_minor: "1000", unit_type: "usd_credit", included_amount: "1" },
  });

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/plans`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as Plan[];
    setItems(next);
    setMessage(`本渠道可见套餐 ${next.length} 个`);
  }

  return (
    <Card className="rounded-stamp border border-hairline bg-canvas-raised  p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道套餐</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">低价或高风险配额会进平台审核。这里只列本渠道 scope。</p>
      <Button variant="outline" onClick={refresh}>
        刷新套餐
      </Button>
      <Form {...form}>
        <form
          className="mt-4 grid max-w-xl gap-2"
          onSubmit={form.handleSubmit(async (values) => {
            const response = await fetch(`${apiBase}/channel/plans`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                name: values.name,
                price_minor: Number(values.price_minor || 0),
                items: [
                  {
                    unit_type: values.unit_type || "usd_credit",
                    included_amount: Number(values.included_amount || 0),
                  },
                ],
              }),
            });
            const body = await response.json();
            if (!response.ok) {
              setCreateMessage(body.error?.message || "创建失败");
              return;
            }
            form.reset();
            setCreateMessage(`已创建 ${body.item?.id} ${body.item?.name} → ${body.item?.status}${body.item?.review_reason ? ` (${body.item.review_reason})` : ""}`);
            await refresh();
          })}
        >
          <h3 className="text-lg font-medium">创建渠道套餐</h3>
          <TextField control={form.control} name="name" label="渠道套餐名" />
          <TextField control={form.control} name="price_minor" label="渠道套餐价格" placeholder="价格 micro-USD，低于 1000000 会审核" />
          <TextField control={form.control} name="unit_type" label="渠道套餐权益单位" placeholder="usd_credit" />
          <TextField control={form.control} name="included_amount" label="渠道套餐权益数量" placeholder="included_amount" />
          <Button size="sm" type="submit">
            创建渠道套餐
          </Button>
          <p className="text-sm text-ink-secondary">{createMessage}</p>
        </form>
      </Form>
      <ul className="mt-3 space-y-2 text-sm text-ink">
        {items.map((item) => (
          <li key={item.id}>
            {item.name} · {item.status} · {item.owner_type} · {item.price_minor ?? 0} micro-USD
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
