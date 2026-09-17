"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { CHANNEL_TYPES, ROLE_TYPES } from "@/lib/tenants";

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

const createChannelSchema = z.object({
  code: z.string().trim().min(1, "请填写渠道 code"),
  type: z.string().trim().min(1, "请选择渠道类型"),
  status: z.string().trim().min(1, "请填写渠道状态"),
  brand_id: z.string().trim(),
});

const createRoleSchema = z.object({
  channel_id: z.string().trim().min(1, "请填写所属租户 ID"),
  role_type: z.string().trim().min(1, "请选择角色类型"),
  parent_id: z.string().trim(),
});

export function CreateChannelDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("新建渠道会复制平台已启用模型白名单。租户不能自建提供商或模型。");
  const form = useForm<z.infer<typeof createChannelSchema>>({
    resolver: zodResolver(createChannelSchema),
    defaultValues: { code: "", type: "B", status: "active", brand_id: "" },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>创建渠道</DialogTitle>
          <DialogDescription>code 要唯一。类型 A/B/C 决定租户能力：A 官方、B 批发、C OEM。创建后用户仍只能靠推广码归因。</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="code" label="渠道 code" />
            <FormField
              control={form.control}
              name="type"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>租户类型</FormLabel>
                  <FormControl>
                    <select className={selectClass} aria-label="租户类型" {...field}>
                      {CHANNEL_TYPES.map((item) => (
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
            <TextField control={form.control} name="status" label="状态" placeholder="active" />
            <TextField control={form.control} name="brand_id" label="品牌 ID" placeholder="默认 brd_official，C 可用 brd_oem" />
            <ConfirmButton
              size="sm"
              title="确认创建渠道"
              description="不要拿种子渠道做实验。创建后只授权平台已启用模型，不能自建提供商。"
              validate={() => form.trigger()}
              onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                try {
                const res = await fetch(`${apiBase}/admin/channels`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify(values),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "创建失败");
                  return false;
                }
                form.reset({ code: "", type: "B", status: "active", brand_id: "" });
                setMessage(`已创建 ${body.item?.id} ${body.item?.code} → ${body.item?.type} / ${body.item?.status}`);
                await queryClient.invalidateQueries();
                onOpenChange(false);
                return true;
                } catch {
                  setMessage(confirmNetworkUnavailable);
                  return false;
                }
})}
            >
              创建渠道
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{message}</p>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

export function CreatePartnerDialog({
  open,
  onOpenChange,
  defaultType,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultType: "agent" | "kol_l1" | "kol_l2";
}) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("层级只能是 agent → kol_l1 → kol_l2。他们属于某个渠道租户，不是独立租户。");
  const form = useForm<z.infer<typeof createRoleSchema>>({
    resolver: zodResolver(createRoleSchema),
    defaultValues: { channel_id: "chn_reseller_b", role_type: defaultType, parent_id: defaultType === "agent" ? "" : "acr_b_agent" },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{defaultType === "agent" ? "新建代理商" : "新建 KOL"}</DialogTitle>
          <DialogDescription>2 级 KOL 必须挂在 1 级下面。代理商第一版不能发展下级代理商。</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="channel_id" label="所属租户 ID" placeholder="chn_..." />
            <FormField
              control={form.control}
              name="role_type"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>角色类型</FormLabel>
                  <FormControl>
                    <select className={selectClass} aria-label="角色类型" {...field}>
                      {ROLE_TYPES.filter((item) => (defaultType === "agent" ? item.value === "agent" : item.value !== "agent")).map(
                        (item) => (
                          <option key={item.value} value={item.value}>
                            {item.label}
                          </option>
                        ),
                      )}
                    </select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <TextField control={form.control} name="parent_id" label="上级角色" placeholder={defaultType === "agent" ? "代理商通常无上级" : "acr_..."} />
            <ConfirmButton
              size="sm"
              title="确认创建推广角色"
              description="层级只能是 agent → kol_l1 → kol_l2。"
              validate={() => form.trigger()}
              onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                try {
                const res = await fetch(`${apiBase}/admin/acquisition-roles`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    channel_org_id: values.channel_id,
                    type: values.role_type,
                    parent_id: values.parent_id,
                  }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "创建角色失败");
                  return false;
                }
                setMessage(`已创建角色 ${body.item?.id}（${body.item?.type}）`);
                await queryClient.invalidateQueries();
                onOpenChange(false);
                return true;
                } catch {
                  setMessage(confirmNetworkUnavailable);
                  return false;
                }
})}
            >
              创建推广角色
            </ConfirmButton>
            <p className="text-sm text-ink-secondary">{message}</p>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

export function OpenCreateButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <Button size="sm" onClick={onClick}>
      {label}
    </Button>
  );
}
