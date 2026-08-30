"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string };

const createSchema = z.object({
  code: z.string().trim().min(1, "请填写渠道 code"),
  type: z.string().trim().min(1, "请填写渠道类型"),
  status: z.string().trim().min(1, "请填写渠道状态"),
  brand_id: z.string().trim(),
});

const patchSchema = z.object({
  channel_id: z.string().trim().min(1, "请填写渠道 ID"),
  status: z.string().trim(),
  type: z.string().trim(),
  brand_id: z.string().trim(),
});

export default function AdminChannelsPage() {
  const queryClient = useQueryClient();
  const [channelID, setChannelID] = useState("chn_reseller_b");
  const [amount, setAmount] = useState("1000000");
  const [ratioBPS, setRatioBPS] = useState("10000");
  const [message, setMessage] = useState("渠道额度按 micro-USD。发放和扣减都要二次确认。");
  const [channelMessage, setChannelMessage] = useState("创建和改状态都要二次确认。不要停用 chn_official_a / chn_reseller_b / chn_oem_c。");
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { code: "", type: "B", status: "active", brand_id: "" },
  });
  const patchForm = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    defaultValues: { channel_id: "", status: "", type: "", brand_id: "" },
  });

  async function loadQuota() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/${channelID}`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "读取额度失败");
      return;
    }
    const bps = body.quota?.issue_ratio_bps ?? 10000;
    setRatioBPS(String(bps));
    setMessage(`可用额度 ${body.quota?.available_minor}，换算比 ${bps} BPS`);
  }

  async function saveRatio() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/${channelID}/issue-rule`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ issue_ratio_bps: Number(ratioBPS) }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已保存换算比 ${body.rule?.issue_ratio_bps} BPS` : body.error?.message || "保存换算比失败");
  }

  async function grant() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/grant`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ channel_org_id: channelID, amount_minor: Number(amount) }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已调整 ${channelID}，可用 ${body.quota?.available_minor}` : body.error?.message || "调整失败");
  }

  return (
    <AdminShell>
      <section className="rounded-stamp border border-hairline bg-canvas-raised  p-6">
        <h2 className="mb-3 text-xl font-medium">渠道额度</h2>
        <p className="mb-3 text-sm text-ink-secondary">B/C 渠道可用额度在用户充值时按平台换算比发放，默认 1:1。正数授予，负数扣减。额度不足时不能再给新用户发放，预授权也会失败。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-56" value={channelID} onChange={(e) => setChannelID(e.target.value)} aria-label="渠道 ID" placeholder="chn_..." />
          <Input className="w-40" value={amount} onChange={(e) => setAmount(e.target.value)} aria-label="额度 micro-USD" placeholder="amount_minor" />
          <Button size="sm" variant="outline" onClick={loadQuota}>
            读取额度
          </Button>
          <ConfirmButton size="sm" title="确认调整额度" description="发放和扣减都会写审计，并从渠道 available 记账。" onConfirm={grant}>
            调整额度
          </ConfirmButton>
        </div>
        <h3 className="mb-2 mt-4 text-lg font-medium">换算比</h3>
        <p className="mb-3 text-sm text-ink-secondary">10000 BPS = 1:1。只有平台/财务能改，B/C 代理商不能改。合法范围 1000–100000（0.1x–10x）。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-40" value={ratioBPS} onChange={(e) => setRatioBPS(e.target.value)} aria-label="换算比 BPS" placeholder="10000" />
          <ConfirmButton size="sm" title="确认保存换算比" description="之后该渠道的充值会按新比例发放额度。" onConfirm={saveRatio}>
            保存换算比
          </ConfirmButton>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <Form {...createForm}>
        <form className="rounded-stamp border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <h2 className="mb-3 text-xl font-medium">创建渠道</h2>
          <p className="mb-3 text-sm text-ink-secondary">code 要唯一。类型 A/B/C。品牌默认官方站。创建后用户仍只能靠推广码归因，不能自助改渠道。</p>
          <div className="mb-3 grid max-w-xl gap-2">
            <TextField control={createForm.control} name="code" label="创建用渠道 code" />
            <TextField control={createForm.control} name="type" label="创建用渠道类型" placeholder="创建用渠道类型 B" />
            <TextField control={createForm.control} name="status" label="创建用渠道状态" placeholder="创建用渠道状态 active" />
            <TextField control={createForm.control} name="brand_id" label="创建用品牌 ID" placeholder="创建用品牌 ID brd_official" />
          </div>
          <ConfirmButton
            size="sm"
            title="确认创建渠道"
            description="不要拿种子渠道做实验。创建后用户仍只能靠推广码归因。"
            validate={() => createForm.trigger()}
            onConfirm={createForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/channels`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify(values),
              });
              const body = await res.json();
              if (!res.ok) {
                setChannelMessage(body.error?.message || "创建失败");
                return;
              }
              createForm.reset();
              setChannelMessage(`已创建 ${body.item?.id} ${body.item?.code} → ${body.item?.type} / ${body.item?.status}`);
              await queryClient.invalidateQueries();
            })}
          >
            创建渠道
          </ConfirmButton>
        </form>
      </Form>
      <Form {...patchForm}>
        <form className="rounded-stamp border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <h2 className="mb-3 text-xl font-medium">改渠道状态</h2>
          <p className="mb-3 text-sm text-ink-secondary">只改状态、类型或品牌。停用后冻结新消费（聊天/媒体 403），余额和历史仍保留。不要停用官方/代理商/OEM 种子渠道。</p>
          <div className="mb-3 grid max-w-xl gap-2">
            <TextField control={patchForm.control} name="channel_id" label="改状态用渠道 ID" />
            <TextField control={patchForm.control} name="status" label="改状态用状态" placeholder="改状态用状态 disabled" />
            <TextField control={patchForm.control} name="type" label="改状态用类型" placeholder="改状态用类型 B" />
            <TextField control={patchForm.control} name="brand_id" label="改状态用品牌 ID" />
          </div>
          <ConfirmButton
            size="sm"
            title="确认保存渠道"
            description="停用后冻结新消费。不要停用 chn_official_a / chn_reseller_b / chn_oem_c。"
            validate={() => patchForm.trigger()}
            onConfirm={patchForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/channels/${values.channel_id}`, {
                method: "PATCH",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({
                  status: values.status,
                  type: values.type,
                  brand_id: values.brand_id,
                }),
              });
              const body = await res.json();
              if (!res.ok) {
                setChannelMessage(body.error?.message || "保存失败");
                return;
              }
              setChannelMessage(`已保存 ${body.item?.id} → ${body.item?.status} / ${body.item?.type}`);
              await queryClient.invalidateQueries();
            })}
          >
            保存渠道
          </ConfirmButton>
        </form>
      </Form>
      <p className="text-sm text-ink-secondary">{channelMessage}</p>
      <AdminListPanel<Channel>
        path="/admin/channels"
        title="渠道 / 代理商"
        columns={[
          { accessorKey: "code", header: "Code" },
          { accessorKey: "type", header: "Type" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "brand_id", header: "Brand" },
        ]}
      />
    </AdminShell>
  );
}
