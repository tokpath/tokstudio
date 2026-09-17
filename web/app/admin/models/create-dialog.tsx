"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { ProviderSlugCombobox } from "@/components/provider-slug-combobox";
import { TextField } from "@/components/text-field";
import { VendorCombobox } from "@/components/vendor-combobox";
import { Form } from "@/components/ui/form";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { CATALOG_LABEL, slugifyCatalogId, suggestPublicId, suggestPublicIdFromDisplay } from "@/lib/catalog-copy";

const createSchema = z
  .object({
    display_name: z.string().trim().min(1, "请填写显示名"),
    vendor: z.string().trim().min(1, "请选择原厂"),
    public_id: z.string().trim().min(1, "请填写公开模型标识"),
    provider_id: z.string().trim(),
    upstream_model_id: z.string().trim(),
  })
  .superRefine((values, ctx) => {
    if (values.provider_id && !values.upstream_model_id) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["upstream_model_id"],
        message: "选定提供商后需要填写上游模型标识",
      });
    }
    if (values.upstream_model_id && !values.provider_id) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["provider_id"],
        message: "填写上游模型标识前请先选择提供商",
      });
    }
  });

export function CreateModelDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: () => void;
}) {
  const queryClient = useQueryClient();
  const previousVendor = useRef("");
  const previousSlug = useRef("");
  const [message, setMessage] = useState("");
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: {
      display_name: "",
      vendor: "",
      public_id: "",
      provider_id: "",
      upstream_model_id: "",
    },
  });
  const displayName = form.watch("display_name");
  const vendor = form.watch("vendor");
  const providersQuery = useQuery({
    queryKey: ["/admin/providers"],
    queryFn: () => apiClient<{ items?: { id: string; name: string; slug: string }[] }>("GET", "/admin/providers"),
    enabled: open,
  });

  useEffect(() => {
    if (!open) {
      return;
    }
    const current = form.getValues("public_id");
    const afterVendor = suggestPublicId(vendor, current, previousVendor.current);
    const next = suggestPublicIdFromDisplay(vendor, displayName, afterVendor, previousSlug.current);
    if (next !== current) {
      form.setValue("public_id", next);
    }
    previousVendor.current = vendor;
    previousSlug.current = slugifyCatalogId(displayName);
  }, [displayName, form, open, vendor]);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          form.reset();
          previousVendor.current = "";
          previousSlug.current = "";
          setMessage("");
        }
        onOpenChange(next);
      }}
    >
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>创建模型</DialogTitle>
          <DialogDescription className="sr-only">填写显示名和原厂，可选提供商。创建后需另一人审核发布。</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="display_name" label={CATALOG_LABEL.displayName} placeholder="例如 HappyHorse 1.0" />
            <VendorCombobox control={form.control} name="vendor" extra={[form.watch("vendor")]} />
            <TextField
              control={form.control}
              name="public_id"
              label={CATALOG_LABEL.publicModelId}
              placeholder="例如 alibaba/happyhorse-1.0"
            />
            <ProviderSlugCombobox
              control={form.control}
              name="provider_id"
              label={CATALOG_LABEL.provider}
              options={providersQuery.data?.items ?? []}
              placeholder="可选"
            />
            <TextField
              control={form.control}
              name="upstream_model_id"
              label={CATALOG_LABEL.upstreamModelId}
              placeholder="可选"
            />
            <ConfirmButton
              size="sm"
              title="确认创建模型"
              description="将创建为待审核草稿。请勿修改 tokenhub/echo-1。"
              validate={() => form.trigger()}
              onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                try {
                const created = await fetch(`${apiBase}/admin/models`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    public_id: values.public_id,
                    vendor: values.vendor,
                    display_name: values.display_name,
                  }),
                });
                const createdBody = await created.json();
                if (!created.ok) {
                  setMessage(createdBody.error?.message || "创建失败");
                  return false;
                }
                let extra = "";
                if (values.provider_id && values.upstream_model_id) {
                  const attached = await fetch(`${apiBase}/admin/models/attach`, {
                    method: "POST",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: JSON.stringify({
                      public_id: values.public_id,
                      provider_id: values.provider_id,
                      upstream_model_id: values.upstream_model_id,
                    }),
                  });
                  const attachedBody = await attached.json();
                  extra = attached.ok
                    ? `，已接到 ${values.provider_id}`
                    : `，但提供商未接上：${attachedBody.error?.message || "关联失败"}`;
                }
                form.reset();
                previousVendor.current = "";
                previousSlug.current = "";
                setMessage(`已创建 ${createdBody.item?.id}${extra}`);
                onOpenChange(false);
                onCreated?.();
                await queryClient.invalidateQueries();
                return true;
                } catch {
                  setMessage(confirmNetworkUnavailable);
                  return false;
                }
})}
            >
              创建模型
            </ConfirmButton>
            {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
