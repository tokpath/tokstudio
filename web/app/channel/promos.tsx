"use client";

import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Promo = { id?: string; code?: string; status?: string; acquisition_role_id?: string };

const schema = z.object({
  code: z.string().trim().min(1, "请填写推广码"),
});

export default function ChannelPromos() {
  const [items, setItems] = useState<Promo[]>([]);
  const [message, setMessage] = useState("推广链接只属于本渠道。用户注册时服务端会固化归因。");
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { code: "" },
  });

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/promotion-codes`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as Promo[];
    setItems(next);
    setMessage(`本渠道推广码 ${next.length} 个`);
  }

  const origin = typeof window === "undefined" ? "" : window.location.origin;

  return (
    <Card>
      <CardTitle className="mb-3 text-xl font-medium">推广链接</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">把推广码发给用户，或复制带 promo 参数的登录链接。</p>
      <Form {...form}>
        <form className="mb-3 flex flex-wrap items-end gap-2" onSubmit={(event) => event.preventDefault()}>
          <TextField control={form.control} name="code" label="新推广码" placeholder="新推广码 THB-SALE" showLabel={false} className="max-w-xs" />
          <ConfirmButton
            variant="outline"
            title="确认创建推广码"
            description="这个码只属于本渠道。用户注册时服务端会固化归因。"
            validate={() => form.trigger()}
            onConfirm={form.handleSubmit(async (values) => {
              const response = await fetch(`${apiBase}/channel/promotion-codes`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ code: values.code }),
              });
              const body = await response.json();
              if (!response.ok) {
                setMessage(body.error?.message || "创建失败");
                return;
              }
              form.reset();
              await refresh();
              setMessage(`已创建推广码 ${body.item?.code || ""}`);
            })}
          >
            创建推广码
          </ConfirmButton>
          <Button variant="outline" onClick={refresh}>
            刷新推广码
          </Button>
        </form>
      </Form>
      <ul className="mt-3 space-y-2 text-sm text-ink">
        {items.map((item) => (
          <li key={item.id}>
            {item.code} · {item.status} · {origin}/login?promo={item.code}
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
