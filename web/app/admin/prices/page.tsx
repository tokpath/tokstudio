"use client";

import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { SealConfirm } from "@/components/seal-confirm";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { Button } from "@/components/ui/button";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import { priceBookColumns, publishedPriceLabel, type PriceBook } from "@/lib/price-book";
import { millionDim } from "@/lib/token-price";

const schema = z.object({
  model: z.string().trim().min(1, "请填写模型 ID"),
  input: z.string().trim().min(1, "请填写终端售价输入"),
  output: z.string().trim().min(1, "请填写终端售价输出"),
  wholesale_input: z.string(),
  wholesale_output: z.string(),
  upstream_cost_input: z.string(),
  upstream_cost_output: z.string(),
  channel_input: z.string(),
  channel_output: z.string(),
});

export default function AdminPricesPage() {
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      model: "tokenhub/echo-1",
      input: "1",
      output: "2",
      wholesale_input: "0.7",
      wholesale_output: "1.4",
      upstream_cost_input: "0.4",
      upstream_cost_output: "0.8",
      channel_input: "",
      channel_output: "",
    },
  });
  const [message, setMessage] = useState("新价格只影响之后的请求。历史版本是只读快照，数字不会被改写。发布需要盖章确认。");

  async function downloadCSV() {
    const res = await fetch(`${apiBase}/admin/price-books?format=csv`, { credentials: "include" });
    const text = await res.text();
    if (!res.ok) {
      setMessage("导出失败");
      return;
    }
    const blob = new Blob([text], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "price-books.csv";
    a.click();
    URL.revokeObjectURL(url);
    setMessage("已导出价格书 CSV（单价列为内部美元/token）。");
  }

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="prices" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">
          发布新版本会把当前 published 标成 superseded。历史 usage 仍按当时快照计费，旧行数字不会动画改写。单个模型也可以在模型编辑页改价。Token
          价按每百万 token 的美元填写，例如 2 表示 $2/M。发布时换算成内部美元/token。列表里的 Token 价也按 /M 显示。
        </p>
        <IfCan action="prices.write">
        <Form {...form}>
          <form className="mb-3 grid gap-2" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="model" label="模型 ID" placeholder="public model id" className="max-w-sm" />
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              <TextField control={form.control} name="upstream_cost_input" label="成本 输入" suffix="美元/M" />
              <TextField control={form.control} name="upstream_cost_output" label="成本 输出" suffix="美元/M" />
              <TextField control={form.control} name="wholesale_input" label="批发 输入" suffix="美元/M" />
              <TextField control={form.control} name="wholesale_output" label="批发 输出" suffix="美元/M" />
              <TextField control={form.control} name="input" label="售价 输入" suffix="美元/M" />
              <TextField control={form.control} name="output" label="售价 输出" suffix="美元/M" />
              <TextField control={form.control} name="channel_input" label="渠道覆盖 输入" suffix="美元/M" />
              <TextField control={form.control} name="channel_output" label="渠道覆盖 输出" suffix="美元/M" />
            </div>
            <SealConfirm
              size="sm"
              title="新牌价只约束之后的请求，已入账金额不会改写。"
              description="当前 published 会标成 superseded。历史版本保持只读快照。"
              validate={() => form.trigger()}
              onConfirm={form.handleSubmit(async (values) => {
                let payload: Record<string, unknown>;
                try {
                  payload = {
                    model: values.model,
                    currency: "USD",
                    customer_sell: millionDim(values.input, values.output),
                  };
                  const wholesale = millionDim(values.wholesale_input, values.wholesale_output);
                  const upstream = millionDim(values.upstream_cost_input, values.upstream_cost_output);
                  const channel = millionDim(values.channel_input, values.channel_output);
                  if (wholesale) {
                    payload.wholesale = wholesale;
                  }
                  if (upstream) {
                    payload.upstream_cost = upstream;
                  }
                  if (channel) {
                    payload.channel_override = channel;
                  }
                } catch (err) {
                  setMessage(err instanceof Error ? err.message : "单价无效");
                  return;
                }
                const res = await fetch(`${apiBase}/admin/price-books`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify(payload),
                });
                const body = await res.json();
                setMessage(res.ok ? `已发布 ${publishedPriceLabel(body.price, values.model)}` : body.error?.message || "发布失败");
              })}
            >
              发布价格
            </SealConfirm>
          </form>
        </Form>
        </IfCan>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <AdminListPanel<PriceBook>
        path="/admin/price-books"
        title="价格版本"
        columns={priceBookColumns}
        actions={
          <Button type="button" size="sm" variant="outline" onClick={downloadCSV}>
            导出 CSV
          </Button>
        }
        emptyTitle="还没有价格版本"
        emptyDetail="发布后会出现版本号、生效时间和四列单价。"
      />
    </AdminShell>
  );
}
