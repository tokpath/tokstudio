"use client";

import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { EmptyState } from "@/components/empty-state";
import { ScrollTable } from "@/components/ui/scroll-table";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { USD_CREDIT, formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { planAudienceText, planPeriodLabel, planQuotaText, planStatusLabel } from "@/lib/plan-display";
import { statusTone } from "@/lib/status-copy";
import { Badge } from "@/components/ui/badge";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import { listEligiblePlanChannels, listPlanChannels } from "@/lib/plan-channels";

type Plan = {
  id: string;
  name: string;
  owner_type: string;
  owner_id: string;
  price_minor: number;
  billing_period: string;
  channel_scope: string;
  channel_ids?: string[];
  status: string;
  items?: { unit_type: string; included_amount: number }[];
};

type ListResponse = { items?: Plan[]; error?: { message?: string } };
type PlanAction = "approve" | "archive" | "reject";

const createSchema = z.object({
  name: z.string().trim().min(1, "请填写套餐名"),
  price_usd: z.string().trim().min(1, "请填写价格"),
  included_usd: z.string().trim().min(1, "请填写额度"),
  billing_period: z.enum(["lifetime", "monthly", "quarterly", "yearly"]),
});

const filterControlClass = "h-10 rounded-control border border-hairline bg-canvas px-3 text-sm";

export default function AdminPlansPage() {
  const [status, setStatus] = useState("");
  const [channelFilter, setChannelFilter] = useState("");
  const [periodFilter, setPeriodFilter] = useState("");
  const [nameInput, setNameInput] = useState("");
  const [nameFilter, setNameFilter] = useState("");
  const [channelScope, setChannelScope] = useState("all");
  const [selectedChannels, setSelectedChannels] = useState<string[]>([]);
  const [channelSearch, setChannelSearch] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [message, setMessage] = useState("");
  const creatingRef = useRef(false);
  const queryClient = useQueryClient();
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  if (channelFilter) params.set("channel_id", channelFilter);
  if (periodFilter) params.set("billing_period", periodFilter);
  if (nameFilter) params.set("name", nameFilter);
  const path = `/admin/plans?${params.toString()}`;
  const query = useQuery({ queryKey: [path], queryFn: () => apiClient<ListResponse>("GET", path) });
  const channelsQuery = useQuery({ queryKey: ["/admin/channels", "plans"], queryFn: listPlanChannels });
  const channels = channelsQuery.data ?? [];
  const eligibleQuery = useQuery({ queryKey: ["/admin/plans/eligible-channels"], queryFn: listEligiblePlanChannels, enabled: createOpen });
  const eligibleChannels = eligibleQuery.data ?? [];
  const channelNames = new Map(channels.map((channel) => [channel.id, channel.code]));
  const items = query.data?.items ?? [];
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { name: "", price_usd: "1", included_usd: "1", billing_period: "lifetime" },
  });

  async function changeStatus(plan: Plan, action: PlanAction): Promise<boolean> {
    const label = action === "approve" ? "发布" : action === "archive" ? "下架" : "拒绝";
    try {
      const res = await fetch(`${apiBase}/admin/plans/${plan.id}${action === "archive" ? "" : "/review"}`, {
        method: action === "archive" ? "PATCH" : "POST",
        credentials: "include",
        headers: confirmHeaders,
        body: JSON.stringify(action === "archive" ? { status: "archived" } : { action }),
      });
      const body = await res.json();
      setMessage(res.ok ? `已${label} ${plan.name}` : body.error?.message || `${label}失败`);
      if (res.ok) await queryClient.invalidateQueries({ queryKey: [path] });
      return res.ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  async function createPlan(values: z.infer<typeof createSchema>) {
    if (creatingRef.current) return;
    const price = parseUsdToMinor(values.price_usd);
    const included = parseUsdToMinor(values.included_usd);
    if (price == null || included == null || price <= 0 || included <= 0) {
      setMessage("请填写大于零的美元金额");
      return;
    }
    if (channelScope === "selected" && selectedChannels.length === 0) {
      setMessage("请选择至少一个渠道");
      return;
    }
    creatingRef.current = true;
    setMessage("");
    try {
      const res = await fetch(`${apiBase}/admin/plans`, {
        method: "POST", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({
          name: values.name, owner_type: "platform", price_minor: price,
          billing_period: values.billing_period, auto_renew_allowed: values.billing_period !== "lifetime",
          channel_scope: channelScope, channel_ids: channelScope === "selected" ? selectedChannels : [],
          items: [{ unit_type: USD_CREDIT, included_amount: included }],
        }),
      });
      const body = await res.json();
      if (!res.ok) {
        setMessage(body.error?.message || "创建失败");
        return;
      }
      createForm.reset();
      setChannelScope("all");
      setSelectedChannels([]);
      setChannelSearch("");
      setMessage(`已创建 ${body.item?.name}，等待发布`);
      setCreateOpen(false);
      void queryClient.invalidateQueries();
    } catch {
      setMessage(confirmNetworkUnavailable);
    } finally {
      creatingRef.current = false;
    }
  }

  return (
    <AdminShell>
      <header className="flex flex-wrap items-center justify-between gap-3">
        <AdminH2 level={1} k="planReview" className="text-2xl font-semibold tracking-tight" />
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="outline" size="sm" onClick={() => void query.refetch()}>刷新套餐</Button>
          <IfCan action="plans.write">
            <Button size="sm" onClick={() => setCreateOpen(true)}>创建套餐</Button>
          </IfCan>
        </div>
      </header>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <div className="mb-4 flex flex-wrap items-center gap-3">
          <select aria-label="按状态筛选" className={filterControlClass} value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="">全部状态</option>
            {Object.entries(planStatusLabel).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
          <select aria-label="按渠道筛选" className={filterControlClass} value={channelFilter} onChange={(event) => setChannelFilter(event.target.value)}>
            <option value="">全部渠道</option>
            {channels.map((channel) => <option key={channel.id} value={channel.id}>{channel.code}</option>)}
          </select>
          <select aria-label="按套餐类型筛选" className={filterControlClass} value={periodFilter} onChange={(event) => setPeriodFilter(event.target.value)}>
            <option value="">全部周期</option>
            <option value="lifetime">不限时</option>
            <option value="monthly">包月</option>
            <option value="quarterly">季付</option>
            <option value="yearly">年付</option>
          </select>
          <form className="flex items-center gap-2" onSubmit={(event) => { event.preventDefault(); setNameFilter(nameInput.trim()); }}>
            <input aria-label="按套餐名筛选" placeholder="套餐名" className={`${filterControlClass} w-44 placeholder:text-ink-secondary focus:placeholder:text-transparent`} value={nameInput} onChange={(event) => setNameInput(event.target.value)} />
            <Button type="submit" variant="outline">筛选</Button>
          </form>
        </div>
        {query.data?.error ? <p className="mb-3 text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        <ScrollTable
          columns={[
            { id: "name", header: "套餐", cell: (item) => <span className="whitespace-nowrap">{item.name}</span> },
            { id: "price", header: "价格", cell: (item) => <span className="whitespace-nowrap">{formatUsdMinor(item.price_minor)}</span> },
            { id: "quota", header: "包括额度", cell: (item) => <span className="whitespace-nowrap">{planQuotaText(item.items)}</span> },
            { id: "period", header: "额度周期", cell: (item) => <span className="whitespace-nowrap">{planPeriodLabel[item.billing_period] || item.billing_period}</span> },
            { id: "channels", header: "适用渠道", cell: (item) => <span className="whitespace-nowrap">{planAudienceText(item, channelNames)}</span> },
            { id: "status", header: "状态", cell: (item) => <Badge tone={statusTone(item.status)}>{planStatusLabel[item.status] || item.status}</Badge> },
            {
              id: "actions", header: "操作", className: "w-full text-right", cell: (item) => (
                <IfCan action="plans.write">
                  <div className="flex w-full flex-nowrap justify-end gap-1">
                    <ConfirmButton size="sm" className="px-2" disabled={!["pending_review", "rejected", "archived"].includes(item.status)} title="确认发布套餐" description={`发布后用户可购买「${item.name}」。`} onConfirm={() => changeStatus(item, "approve")}>发布</ConfirmButton>
                    <ConfirmButton size="sm" className="px-2" variant="outline" disabled={item.status !== "published"} title="确认下架套餐" description={`下架「${item.name}」后停止新购买，已有权益保留。`} onConfirm={() => changeStatus(item, "archive")}>下架</ConfirmButton>
                    <ConfirmButton size="sm" className="px-2" variant="outline" disabled={item.status !== "pending_review"} title="确认拒绝套餐" description={`拒绝「${item.name}」，以后仍可重新发布。`} onConfirm={() => changeStatus(item, "reject")}>拒绝</ConfirmButton>
                  </div>
                </IfCan>
              ),
            },
          ]}
          rows={items}
          stickyEnds={false}
          minWidthClassName="min-w-[64rem]"
          getRowId={(item) => item.id}
          empty={<EmptyState title="暂无套餐" detail="" />}
        />
        {message ? <p role="status" className="mt-3 text-sm text-ink-secondary">{message}</p> : null}
      </section>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto" aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle>创建套餐</DialogTitle>
          </DialogHeader>
          <Form {...createForm}>
            <form className="grid gap-3" onSubmit={createForm.handleSubmit(createPlan)}>
              <TextField control={createForm.control} name="name" label="套餐名称" />
              <TextField control={createForm.control} name="price_usd" label="售价" placeholder="1.00" suffix="USD" />
              <TextField control={createForm.control} name="included_usd" label="包含额度" placeholder="1.00" suffix="USD" />
              <label className="grid gap-1 text-sm" htmlFor="billing-period">
                额度周期
                <select id="billing-period" className="h-10 rounded-md border border-hairline bg-canvas-raised px-3" {...createForm.register("billing_period")}>
                  <option value="lifetime">不限时</option>
                  <option value="monthly">包月</option>
                  <option value="quarterly">季付</option>
                  <option value="yearly">年付</option>
                </select>
              </label>
              <fieldset className="grid gap-2 text-sm">
                <legend>适用渠道</legend>
                <label className="flex items-center gap-2"><input type="radio" checked={channelScope === "all"} onChange={() => setChannelScope("all")} />所有渠道</label>
                <label className="flex items-center gap-2"><input type="radio" checked={channelScope === "selected"} onChange={() => setChannelScope("selected")} />指定渠道</label>
                {channelScope === "selected" ? <div className="grid gap-2">
                  <input aria-label="查找适用渠道" className="h-9 rounded-md border border-hairline bg-canvas-raised px-2" placeholder="查找渠道" value={channelSearch} onChange={(event) => setChannelSearch(event.target.value)} />
                  <div className="max-h-32 overflow-y-auto rounded-md border border-hairline p-2">
                    {eligibleChannels.filter((channel) => channel.code.toLowerCase().includes(channelSearch.toLowerCase())).map((channel) => <label key={channel.id} className="flex items-center gap-2 py-1"><input type="checkbox" checked={selectedChannels.includes(channel.id)} onChange={(event) => setSelectedChannels(event.target.checked ? [...selectedChannels, channel.id] : selectedChannels.filter((id) => id !== channel.id))} />{channel.code}</label>)}
                    {!eligibleChannels.length ? <span className="text-ink-secondary">暂无下属渠道</span> : null}
                  </div>
                  <span className="text-xs text-ink-secondary">已选 {selectedChannels.length} 个渠道</span>
                </div> : null}
              </fieldset>
              {message ? <p role="status" className="text-sm text-danger">{message}</p> : null}
              <Button size="sm" type="submit" disabled={createForm.formState.isSubmitting}>创建套餐</Button>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}
