"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { AdminSelectField } from "@/components/admin-select-field";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { protocolOptions, providerHref } from "@/lib/catalog-admin";
import { CATALOG_LABEL, slugifyCatalogId, suggestProviderSlug } from "@/lib/catalog-copy";

const createSchema = z.object({
  name: z.string().trim().min(1, "请填写名称"),
  slug: z.string().trim().min(1, "请填写提供商标识"),
  kind: z.enum(["direct", "aggregator"]),
  adapter: z.string().trim().min(1, "请选择协议"),
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
  const previousSlug = useRef("");
  const [message, setMessage] = useState("提供商是进货渠道。创建后去详情页填密钥，再到模型页接到公开模型。");
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { name: "", slug: "", kind: "direct", adapter: "openai", base_url: "" },
  });
  const name = form.watch("name");

  useEffect(() => {
    if (!open) {
      return;
    }
    const current = form.getValues("slug");
    const next = suggestProviderSlug(name, current, previousSlug.current);
    if (next !== current) {
      form.setValue("slug", next);
    }
    previousSlug.current = slugifyCatalogId(name);
  }, [form, name, open]);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          form.reset({ name: "", slug: "", kind: "direct", adapter: "openai", base_url: "" });
          previousSlug.current = "";
          setMessage("提供商是进货渠道。创建后去详情页填密钥，再到模型页接到公开模型。");
        }
        onOpenChange(next);
      }}
    >
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
            <TextField control={form.control} name="slug" label={CATALOG_LABEL.providerSlug} placeholder="例如 openai，创建后不要改" />
            <p className="text-sm text-ink-secondary">会按名称自动生成，创建后不要改。</p>
            <AdminSelectField
              control={form.control}
              name="kind"
              label="类型"
              options={[
                { value: "direct", label: "直连 · 官方或兼容协议" },
                { value: "aggregator", label: "聚合 · OpenRouter 等，禁止回流 TokenHub" },
              ]}
            />
            <AdminSelectField
              control={form.control}
              name="adapter"
              label="协议"
              options={protocolOptions(form.watch("adapter"))}
            />
            <p className="text-sm text-ink-secondary">
              请求怎么发给上游。同一协议可接多家（例如官方 OpenAI 和百炼都选 OpenAI 兼容）。
            </p>
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
                previousSlug.current = "";
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
