"use client";

import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction } from "@/lib/rbac";

import { useEffect, useMemo, useRef, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { ListResourceView } from "@/components/console/list-resource-view";
import { SubmitStatus } from "@/components/console/submit-status";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Form } from "@/components/ui/form";
import { ScrollTable } from "@/components/ui/scroll-table";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { fetchListItems } from "@/lib/list-resource";
import { USD_CREDIT, formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { planAudienceText, planPeriodLabel, planQuotaText, planStatusLabel } from "@/lib/plan-display";
import { statusTone } from "@/lib/status-copy";
import { confirmJsonAction } from "@/lib/submit-result";
import { listEligiblePlanChannels, type PlanChannel } from "@/lib/plan-channels";

type Plan = {
  id?: string;
  name?: string;
  status?: string;
  owner_type?: string;
  owner_id?: string;
  price_minor?: number;
  billing_period?: string;
  channel_scope?: string;
  channel_ids?: string[];
  items?: { unit_type?: string; included_amount?: number }[];
  review_reason?: string;
};

export function ChannelPlans() {
  const permissionViewer = useViewer();
  const canOperate = canChannelAction("operations", permissionViewer);
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [createOpen, setCreateOpen] = useState(false);
  const [createError, setCreateError] = useState("");
  const [creating, setCreating] = useState(false);
  const [statusFilter, setStatusFilter] = useState("");
  const [channelFilter, setChannelFilter] = useState("");
  const [periodFilter, setPeriodFilter] = useState("");
  const [nameInput, setNameInput] = useState("");
  const [nameFilter, setNameFilter] = useState("");
  const [channels, setChannels] = useState<PlanChannel[]>([]);
  const [channelScope, setChannelScope] = useState("all");
  const [selectedChannels, setSelectedChannels] = useState<string[]>([]);
  const [channelSearch, setChannelSearch] = useState("");
  const [actionMessage, setActionMessage] = useState("");
  const creatingRef = useRef(false);
  const submitGen = useRef(0);
  const planQuery = new URLSearchParams();
  if (statusFilter) planQuery.set("status", statusFilter);
  if (channelFilter) planQuery.set("channel_id", channelFilter);
  if (periodFilter) planQuery.set("billing_period", periodFilter);
  if (nameFilter) planQuery.set("name", nameFilter);
  const planQueryString = planQuery.toString();
  const list = useListResource<Plan>({
    queryKey: planQueryString,
    load: () => fetchListItems(`${apiBase}/channel/plans${planQueryString ? `?${planQueryString}` : ""}`),
  });
  const schema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("nameRequired")),
        price_usd: z.string().trim().min(1, t("priceRequired")),
        included_usd: z.string().trim().min(1, t("amountRequired")),
        billing_period: z.enum(["lifetime", "monthly", "quarterly", "yearly"]),
      }),
    [t],
  );
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", price_usd: "1", included_usd: "1", billing_period: "lifetime" },
  });

  useEffect(() => {
    void listEligiblePlanChannels().then(setChannels).catch(() => setChannels([]));
  }, []);

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
              auto_renew_allowed: values.billing_period !== "lifetime",
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
        onSuccess: async () => {
          if (generation !== submitGen.current) {
            return;
          }
          form.reset();
          setChannelScope("all");
          setSelectedChannels([]);
          setChannelSearch("");
          setCreateError("");
          setCreateOpen(false);
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

  const channelNames = new Map(channels.map((channel) => [channel.id, channel.code]));

  return (
    <div className="flex flex-col gap-6">
    <I18nConsoleHeader id="channelPlans" hideDescription actions={<>
      <Button variant="outline" size="sm" onClick={() => void list.reload()}>{t("refreshPlans")}</Button>
      {canOperate ? <Button size="sm" onClick={() => setCreateOpen(true)}>{t("createPlan")}</Button> : null}
    </>} />
    <Card>
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <label className="grid gap-1 text-sm">状态
          <select aria-label="按状态筛选" className="h-10 rounded-control border border-hairline bg-canvas px-3 text-sm" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
            <option value="">全部状态</option>
            <option value="pending_review">待审核</option>
            <option value="published">已发布</option>
            <option value="archived">已下架</option>
            <option value="rejected">已拒绝</option>
          </select>
        </label>
        <label className="grid gap-1 text-sm">渠道
          <select aria-label="按渠道筛选" className="h-10 rounded-control border border-hairline bg-canvas px-3 text-sm" value={channelFilter} onChange={(event) => setChannelFilter(event.target.value)}>
            <option value="">全部渠道</option>
            {channels.map((channel) => <option key={channel.id} value={channel.id}>{channel.code}</option>)}
          </select>
        </label>
        <label className="grid gap-1 text-sm">套餐类型
          <select aria-label="按套餐类型筛选" className="h-10 rounded-control border border-hairline bg-canvas px-3 text-sm" value={periodFilter} onChange={(event) => setPeriodFilter(event.target.value)}>
            <option value="">全部方式</option>
            <option value="lifetime">不限时</option>
            <option value="monthly">包月</option>
            <option value="quarterly">季付</option>
            <option value="yearly">年付</option>
          </select>
        </label>
        <form className="flex items-end gap-2" onSubmit={(event) => { event.preventDefault(); setNameFilter(nameInput.trim()); }}>
          <label className="grid gap-1 text-sm">套餐名
            <input aria-label="按套餐名筛选" className="h-10 w-44 rounded-control border border-hairline bg-canvas px-3 text-sm" value={nameInput} onChange={(event) => setNameInput(event.target.value)} />
          </label>
          <Button type="submit" variant="outline">筛选</Button>
        </form>
      </div>
      {actionMessage ? <p role="status" className="mb-3 text-sm text-ink-secondary">{actionMessage}</p> : null}
      {canOperate ? <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto" aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle>{t("createPlan")}</DialogTitle>
          </DialogHeader>
          <Form {...form}>
            <form className="grid gap-3" onSubmit={form.handleSubmit((values) => void createPlan(values))}>
              <TextField control={form.control} name="name" label={t("planName")} />
              <TextField control={form.control} name="price_usd" label={t("planPrice")} placeholder={t("planPricePh")} suffix={tc("usd")} />
              <TextField control={form.control} name="included_usd" label={t("planAmount")} placeholder="1.00" suffix={tc("usd")} />
              <label className="grid gap-1 text-sm" htmlFor="channel-plan-period">
                {t("billingPeriod")}
                <select id="channel-plan-period" className="h-10 rounded-md border border-hairline bg-canvas-raised px-3" {...form.register("billing_period")}>
                  <option value="lifetime">{t("periodLifetime")}</option>
                  <option value="monthly">{t("periodMonthly")}</option>
                  <option value="quarterly">{t("periodQuarterly")}</option>
                  <option value="yearly">{t("periodYearly")}</option>
                </select>
              </label>
              <fieldset className="grid gap-2 text-sm">
                <legend>适用渠道</legend>
                <label className="flex items-center gap-2"><input type="radio" checked={channelScope === "all"} onChange={() => setChannelScope("all")} />所有渠道</label>
                <label className="flex items-center gap-2"><input type="radio" checked={channelScope === "selected"} onChange={() => setChannelScope("selected")} />指定渠道</label>
                {channelScope === "selected" ? <div className="grid gap-2">
                  <input aria-label="查找适用渠道" className="h-9 rounded-md border border-hairline bg-canvas-raised px-2" placeholder="查找渠道" value={channelSearch} onChange={(event) => setChannelSearch(event.target.value)} />
                  <div className="max-h-32 overflow-y-auto rounded-md border border-hairline p-2">
                    {channels.filter((channel) => channel.code.toLowerCase().includes(channelSearch.toLowerCase())).map((channel) => <label key={channel.id} className="flex items-center gap-2 py-1"><input type="checkbox" checked={selectedChannels.includes(channel.id)} onChange={(event) => setSelectedChannels(event.target.checked ? [...selectedChannels, channel.id] : selectedChannels.filter((id) => id !== channel.id))} />{channel.code}</label>)}
                  </div>
                  <span className="text-xs text-ink-secondary">已选 {selectedChannels.length} 个渠道</span>
                </div> : null}
              </fieldset>
              {createError ? <SubmitStatus error={createError} /> : null}
              <Button size="sm" type="submit" disabled={creating}>{creating ? tc("submitting") : t("createPlan")}</Button>
            </form>
          </Form>
        </DialogContent>
      </Dialog> : null}
      <ListResourceView
        snapshot={list.snapshot}
        loadingTitle={t("plansTitle")}
        emptyTitle={t("emptyPlans")}
        emptyDetail=""
        onRetry={() => void list.reload()}
      >
        <ScrollTable
          stickyEnds={false}
          density="admin"
          minWidthClassName="min-w-[64rem]"
          getRowId={(item) => item.id || item.name || "plan"}
          rows={list.snapshot.items}
          columns={[
            { id: "name", header: "套餐", cell: (item) => <span className="whitespace-nowrap">{item.name || "—"}</span> },
            { id: "price", header: "价格", cell: (item) => <span className="whitespace-nowrap">{formatUsdMinor(item.price_minor, tc("lessThanCent"))}</span> },
            { id: "quota", header: "包括额度", cell: (item) => <span className="whitespace-nowrap">{planQuotaText(item.items)}</span> },
            { id: "period", header: "额度周期", cell: (item) => <span className="whitespace-nowrap">{planPeriodLabel[item.billing_period || ""] || item.billing_period || "—"}</span> },
            { id: "channels", header: "适用渠道", cell: (item) => <span className="whitespace-nowrap">{planAudienceText(item, channelNames)}</span> },
            { id: "status", header: "状态", cell: (item) => <Badge tone={statusTone(item.status)}>{planStatusLabel[item.status || ""] || item.status || "—"}</Badge> },
            { id: "actions", header: "操作", className: "w-full text-right", cell: (item) => <span className="flex w-full justify-end gap-1">
              <ConfirmButton size="sm" disabled={!canOperate || !item.id || !["pending_review", "rejected", "archived"].includes(item.status || "")} title="确认发布套餐" description={`发布后本品牌用户可购买「${item.name}」。`} onConfirm={() => changeStatus(item, "approve")}>发布</ConfirmButton>
              <ConfirmButton size="sm" variant="outline" disabled={!canOperate || !item.id || item.status !== "published"} title="确认下架套餐" description={`下架「${item.name}」后停止新购买，已有权益保留。`} onConfirm={() => changeStatus(item, "archive")}>下架</ConfirmButton>
              <ConfirmButton size="sm" variant="outline" disabled={!canOperate || !item.id || item.status !== "pending_review"} title="确认拒绝套餐" description={`拒绝「${item.name}」。`} onConfirm={() => changeStatus(item, "reject")}>拒绝</ConfirmButton>
            </span> },
          ]}
        />
      </ListResourceView>
    </Card>
    </div>
  );
}

export default ChannelPlans;
