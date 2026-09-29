"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { SubmitStatus } from "@/components/console/submit-status";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { fetchListItems } from "@/lib/list-resource";
import { USD_CREDIT, formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { ownerTypeLabelKey, statusLabelKey, statusTone } from "@/lib/status-copy";
import { confirmJsonAction } from "@/lib/submit-result";
import { listEligiblePlanChannels, type PlanChannel } from "@/lib/plan-channels";

type Plan = { id?: string; name?: string; status?: string; owner_type?: string; owner_id?: string; price_minor?: number; review_reason?: string };

export function ChannelPlans() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [createMessage, setCreateMessage] = useState(t("createHint"));
  const [createError, setCreateError] = useState("");
  const [creating, setCreating] = useState(false);
  const [channels, setChannels] = useState<PlanChannel[]>([]);
  const [channelScope, setChannelScope] = useState("all");
  const [selectedChannels, setSelectedChannels] = useState<string[]>([]);
  const [channelSearch, setChannelSearch] = useState("");
  const [actionMessage, setActionMessage] = useState("");
  const creatingRef = useRef(false);
  const submitGen = useRef(0);
  const list = useListResource<Plan>({
    load: () => fetchListItems(`${apiBase}/channel/plans`),
  });
  const schema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("nameRequired")),
        price_usd: z.string().trim().min(1, t("priceRequired")),
        included_usd: z.string().trim().min(1, t("amountRequired")),
        billing_period: z.enum(["once", "monthly", "quarterly", "yearly"]),
      }),
    [t],
  );
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", price_usd: "1", included_usd: "1", billing_period: "once" },
  });

  useEffect(() => {
    void listEligiblePlanChannels().then(setChannels).catch(() => setChannels([]));
  }, []);

  function statusText(status?: string) {
    const key = statusLabelKey(status);
    return key ? tc(key) : status ? tc("stUnknown", { status }) : "—";
  }

  function ownerText(owner?: string) {
    const key = ownerTypeLabelKey(owner);
    return key ? tc(key) : owner || "—";
  }

  async function createPlan(values: z.infer<typeof schema>) {
    if (creatingRef.current) {
      return;
    }
    const price = parseUsdToMinor(values.price_usd);
    const included = parseUsdToMinor(values.included_usd);
    if (price == null || included == null) {
      setCreateError(t("priceRequired"));
      return;
    }
    if (channelScope === "selected" && selectedChannels.length === 0) { setCreateError("请选择至少一个渠道"); return; }
    const generation = ++submitGen.current;
    creatingRef.current = true;
    setCreating(true);
    setCreateError("");
    try {
      return await confirmJsonAction({
        request: () =>
          fetch(`${apiBase}/channel/plans`, {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              name: values.name,
              price_minor: price,
              billing_period: values.billing_period,
              auto_renew_allowed: values.billing_period !== "once",
              channel_scope: channelScope,
              channel_ids: channelScope === "selected" ? selectedChannels : [],
              items: [{ unit_type: USD_CREDIT, included_amount: included }],
            }),
          }),
        failFallback: tc("createFailed"),
        networkMessage: tc("listNetwork"),
        onError: (message) => {
          if (generation !== submitGen.current) {
            return;
          }
          setCreateError(message);
        },
        onSuccess: async (body) => {
          if (generation !== submitGen.current) {
            return;
          }
          const item = (body as { item?: Plan }).item;
          form.reset();
          setChannelScope("all");
          setSelectedChannels([]);
          setChannelSearch("");
          setCreateError("");
          setCreateMessage(
            t("createdPlan", {
              id: item?.id || "",
              name: item?.name || values.name,
              status: `${statusText(item?.status)}${item?.review_reason ? ` (${item.review_reason})` : ""}`,
            }),
          );
          await list.reload();
        },
      });
    } finally {
      creatingRef.current = false;
      setCreating(false);
    }
  }

  async function changeStatus(item: Plan, action: "approve" | "archive" | "reject"): Promise<boolean> {
    if (!item.id) return false;
    try {
      const response = await fetch(`${apiBase}/admin/plans/${encodeURIComponent(item.id)}${action === "archive" ? "" : "/review"}`, {
        method: action === "archive" ? "PATCH" : "POST", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify(action === "archive" ? { status: "archived" } : { action }),
      });
      const body = await response.json();
      if (!response.ok) {
        setActionMessage(body.error?.message || "操作失败");
        return false;
      }
      setActionMessage(`已${action === "approve" ? "发布" : action === "archive" ? "下架" : "拒绝"} ${item.name}`);
      await list.reload();
      return true;
    } catch {
      setActionMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("plansTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("plansLead")}</p>
      <p className="mb-3 text-sm text-ink-secondary">本品牌统一制定套餐和价格，适用于本平台及指定下属渠道。创建后在列表中发布。</p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {t("refreshPlans")}
      </Button>
      {actionMessage ? <p role="status" className="mt-2 text-sm text-ink-secondary">{actionMessage}</p> : null}
      <Form {...form}>
        <form className="mt-4 grid max-w-xl gap-2" onSubmit={form.handleSubmit((values) => void createPlan(values))}>
          <h3 className="text-lg font-medium">{t("createPlan")}</h3>
          <TextField control={form.control} name="name" label={t("planName")} />
          <TextField control={form.control} name="price_usd" label={t("planPrice")} placeholder={t("planPricePh")} suffix={tc("usd")} />
          <p className="text-sm text-ink-secondary">
            {t("planUnit")}：{t("planCreditUsd")}
          </p>
          <TextField control={form.control} name="included_usd" label={t("planAmount")} placeholder="1.00" suffix={tc("usd")} />
          <label className="grid gap-1 text-sm" htmlFor="channel-plan-period">
            {t("billingPeriod")}
            <select id="channel-plan-period" className="h-10 rounded-md border border-hairline bg-canvas-raised px-3" {...form.register("billing_period")}>
              <option value="once">{t("periodOnce")}</option>
              <option value="monthly">{t("periodMonthly")}</option>
              <option value="quarterly">{t("periodQuarterly")}</option>
              <option value="yearly">{t("periodYearly")}</option>
            </select>
          </label>
          <p className="text-xs text-ink-secondary">一次性额度长期有效；周期套餐每期补充额度。</p>
          <fieldset className="grid gap-2 text-sm">
            <legend>适用渠道</legend>
            <label className="flex items-center gap-2"><input type="radio" checked={channelScope === "all"} onChange={() => setChannelScope("all")} />所属及下属所有渠道</label>
            <label className="flex items-center gap-2"><input type="radio" checked={channelScope === "selected"} onChange={() => setChannelScope("selected")} />指定渠道</label>
            {channelScope === "selected" ? <div className="grid gap-2">
              <input aria-label="查找适用渠道" className="h-9 rounded-md border border-hairline bg-canvas-raised px-2" placeholder="查找渠道" value={channelSearch} onChange={(event) => setChannelSearch(event.target.value)} />
              <div className="max-h-32 overflow-y-auto rounded-md border border-hairline p-2">
                {channels.filter((channel) => channel.code.toLowerCase().includes(channelSearch.toLowerCase())).map((channel) => <label key={channel.id} className="flex items-center gap-2 py-1"><input type="checkbox" checked={selectedChannels.includes(channel.id)} onChange={(event) => setSelectedChannels(event.target.checked ? [...selectedChannels, channel.id] : selectedChannels.filter((id) => id !== channel.id))} />{channel.code}</label>)}
              </div>
              <span className="text-xs text-ink-secondary">已选 {selectedChannels.length} 个渠道</span>
            </div> : null}
          </fieldset>
          <Button size="sm" type="submit" disabled={creating}>
            {creating ? tc("submitting") : t("createPlan")}
          </Button>
          {createError ? <SubmitStatus error={createError} /> : <p className="text-sm text-ink-secondary">{createMessage}</p>}
        </form>
      </Form>
      <ListResourceView
        snapshot={list.snapshot}
        loadingTitle={t("plansTitle")}
        emptyTitle={t("emptyPlans")}
        emptyDetail={t("emptyPlansDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colPlan"), t("colStatus"), t("colOwner"), t("colPrice"), "操作"]}
          emptyTitle={t("emptyPlans")}
          emptyDetail={t("emptyPlansDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: item.id || item.name || "plan",
            cells: [
              item.name || "—",
              <Badge key="st" tone={statusTone(item.status)}>
                {statusText(item.status)}
              </Badge>,
              ownerText(item.owner_type),
              formatUsdMinor(item.price_minor, tc("lessThanCent")),
              <span key="actions" className="flex flex-wrap gap-1">
                <ConfirmButton size="sm" disabled={!item.id || !["pending_review", "rejected", "archived"].includes(item.status || "")} title="确认发布套餐" description={`发布后本品牌用户可购买「${item.name}」。`} onConfirm={() => changeStatus(item, "approve")}>发布</ConfirmButton>
                <ConfirmButton size="sm" variant="outline" disabled={!item.id || item.status !== "published"} title="确认下架套餐" description={`下架「${item.name}」后停止新购买，已有权益保留。`} onConfirm={() => changeStatus(item, "archive")}>下架</ConfirmButton>
                <ConfirmButton size="sm" variant="outline" disabled={!item.id || item.status !== "pending_review"} title="确认拒绝套餐" description={`拒绝「${item.name}」。`} onConfirm={() => changeStatus(item, "reject")}>拒绝</ConfirmButton>
              </span>,
            ],
          }))}
        />
      </ListResourceView>
    </Card>
  );
}

export default ChannelPlans;
