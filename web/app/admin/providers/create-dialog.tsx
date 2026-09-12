"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { PROVIDER_ADAPTERS, providerHref } from "@/lib/catalog-admin";

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

const createSchema = z.object({
  name: z.string().trim().min(1, "请填写名称"),
  slug: z.string().trim().min(1, "请填写标识"),
  kind: z.enum(["direct", "aggregator"]),
  adapter: z.string().trim().min(1, "请选择适配器"),
  base_url: z.string().trim(),
});

export function CreateProviderDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("一家提供商可关联多个公开模型，例如 OpenAI 同时提供 gpt-5.6 和 gpt-4.1。创建后请前往详情页填写上游 Key。");
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { name: "", slug: "", kind: "direct", adapter: "openai", base_url: "" },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>接入提供商</DialogTitle>
          <DialogDescription>
            提供商是上游进货渠道，不是客户看到的模型名。直连走官方或兼容协议；聚合走 OpenRouter 这类中转，禁止回流 TokenHub。
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="name" label="名称" placeholder="例如 OpenAI" />
            <TextField control={form.control} name="slug" label="标识" placeholder="例如 openai，创建后不要改" />
            <FormField
              control={form.control}
              name="kind"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>类型</FormLabel>
                  <FormControl>
                    <select className={selectClass} aria-label="类型" {...field}>
                      <option value="direct">直连 · 官方或兼容协议</option>
                      <option value="aggregator">聚合 · OpenRouter 等，禁止回流 TokenHub</option>
                    </select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="adapter"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>适配器</FormLabel>
                  <FormControl>
                    <select className={selectClass} aria-label="适配器" {...field}>
                      {PROVIDER_ADAPTERS.map((item) => (
                        <option key={item.value} value={item.value}>
                          {item.label}
                        </option>
                      ))}
                    </select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <TextField
              control={form.control}
              name="base_url"
              label="上游地址"
              placeholder="https://api.openai.com/v1，沙箱可留空"
            />
            <ConfirmButton
              size="sm"
              title="确认接入提供商"
              description="密钥不会在这一步填写。创建后到详情页轮换凭据。"
              validate={() => form.trigger()}
              onConfirm={form.handleSubmit(async (values) => {
                const res = await fetch(`${apiBase}/admin/providers`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    name: values.name,
                    slug: values.slug,
                    kind: values.kind,
                    adapter: values.adapter,
                    base_url: values.base_url,
                  }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "创建失败");
                  return;
                }
                form.reset({ name: "", slug: "", kind: "direct", adapter: "openai", base_url: "" });
                await queryClient.invalidateQueries();
                onOpenChange(false);
                router.push(providerHref(String(body.item?.slug || body.item?.id || values.slug)));
              })}
            >
              创建
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{message}</p>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
