"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { collection, CommandResultSchema, Decimal, PlanSchema, ReferralSchema, ResourcePageSchema, type Entity } from "@/lib/contracts";
import { compareDecimal } from "@/lib/decimal";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ContextHelp, useCustomerPendingGuard } from "./customer-ui";
import { Empty, ErrorPanel, Heading, Loading, Notice, When, useCommand } from "./shared";

// Shift the decimal point without rounding a customer's concession rate.
function percentage(value: string, toRate = false) {
  const [whole, fraction = ""] = value.split(".");
  if (toRate) return `${BigInt(whole) / 100n}.${(BigInt(whole) % 100n).toString().padStart(2, "0")}${fraction}`.replace(/0+$/, "").replace(/\.$/, "");
  const digits = fraction.padEnd(2, "0");
  return `${BigInt(whole + digits.slice(0, 2))}${digits.slice(2) ? "." + digits.slice(2) : ""}`.replace(/\.0+$/, "");
}
const localTime = (date: Date) => new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

export function CustomerActivities() {
  const rows = useQuery({ queryKey: ["customer-activities"], queryFn: () => request("/api/v1/me/activities/workspace", ResourcePageSchema) });
  const referrals = useQuery({ queryKey: ["referrals"], queryFn: () => request("/api/v1/me/referrals", ReferralSchema) });
  const plans = useQuery({ queryKey: ["purchase-plans"], queryFn: () => request("/api/v1/me/plans/catalog", collection(PlanSchema)) });
  const [editing, setEditing] = useState<Entity | null | undefined>();
  const qualified = referrals.data?.qualified && referrals.data.function_status === "active";
  return <><Heading title="我的 KOL 活动" description="从本人的直接佣金比例中让利，为直属受邀客户提供套餐加赠。" />
    {referrals.isPending || rows.isPending || plans.isPending ? <Loading /> : referrals.isError || rows.isError || plans.isError ? <ErrorPanel error={referrals.error || rows.error || plans.error} retry={() => { void referrals.refetch(); void rows.refetch(); void plans.refetch(); }} /> : <div className="space-y-5">
      {!qualified && <Notice>当前尚未取得可用的 KOL 推广资格。可以在邀请与收益页查看资格金额及功能状态。</Notice>}
      {qualified && editing === undefined && <Button onClick={() => setEditing(null)} disabled={!plans.data.items.length}>创建推广活动</Button>}
      {qualified && editing !== undefined && <ActivityForm key={editing?.id || "new"} item={editing} plans={plans.data.items} maximum={referrals.data.direct_rate} close={() => setEditing(undefined)} />}
      {!rows.data.items.length ? <Empty title="还没有推广活动">取得有效资格后，选择套餐、让利比例及活动期限即可发布。</Empty> : rows.data.items.map(item => <Card key={item.id}><div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="font-semibold">{plans.data.items.find(plan => plan.id === item.values.package_id)?.name || "历史套餐活动"}</h2><p className="mt-3 text-sm text-ink-secondary">让利 {percentage(String(item.values.concession_rate))} 个百分点 · {item.values.enabled ? "已启用" : "已停用"}</p><p className="mt-3 text-xs text-ink-mute"><When value={String(item.values.starts_at)} /> 至 <When value={String(item.values.ends_at)} /></p><p className="mt-3 text-xs text-ink-mute">每位客户最多 {String(item.values.per_customer_limit)} 次 · 总次数{item.values.total_use_limit ? `最多 ${String(item.values.total_use_limit)} 次` : "不单独限制"} · 活动版本 {item.version}</p><p className="mt-3 text-xs text-ink-mute">赠送预算 {String(item.values.gift_budget_atg)} ATG · {({ any: "所有直属受邀客户", first_order: "首次购买客户", existing: "已有购买记录客户" } as Record<string, string>)[String(item.values.eligibility)]}</p></div>{qualified && editing === undefined && <Button variant="outline" onClick={() => setEditing(item)}>修订活动</Button>}</div></Card>)}
      <ContextHelp title="活动让利与组合规则"><p>可让利上限为本人当前直接佣金比例。额外赠送按套餐原始购买额度和套餐自带赠送之和计算，经营方活动加赠不再计入计算基数。</p><p>活动仅对有效直属受邀客户生效。是否可以叠加经营方活动，由经营方明确开放，以客户购买报价为准。修订只影响后续订单，已成交的权益和比例保留。</p></ContextHelp>
    </div>}
  </>;
}

function ActivityForm({ item, plans, maximum, close }: { item: Entity | null; plans: ReturnType<typeof PlanSchema.parse>[]; maximum: string; close: () => void }) {
  const value = item?.values;
  const [plan, setPlan] = useState(String(value?.package_id || plans[0]?.id || ""));
  const [points, setPoints] = useState(value ? percentage(String(value.concession_rate)) : "");
  const [eligibility, setEligibility] = useState(String(value?.eligibility || "any"));
  const [starts, setStarts] = useState(localTime(value ? new Date(String(value.starts_at)) : new Date()));
  const [ends, setEnds] = useState(localTime(value ? new Date(String(value.ends_at)) : new Date(Date.now() + 7 * 86400000)));
  const [perCustomer, setPerCustomer] = useState(String(value?.per_customer_limit || 1));
  const [total, setTotal] = useState(String(value?.total_use_limit || 0));
  const [budget, setBudget] = useState(String(value?.gift_budget_atg || "0"));
  const [enabled, setEnabled] = useState(value ? !!value.enabled : true), [agree, setAgree] = useState(false), [validation, setValidation] = useState<string>();
  const command = useCommand("/api/v1/me/activities", CommandResultSchema);
  const blocked = useCustomerPendingGuard(command.busy || command.unresolved);
  return <Card><form className="space-y-5" onSubmit={async event => {
    event.preventDefault();
    if (command.unresolved) { await command.run(undefined); return; }
    setValidation(undefined);
    if (!/^\d+(\.\d{1,16})?$/.test(points) || compareDecimal(percentage(points, true), "0") <= 0 || compareDecimal(percentage(points, true), maximum) > 0) { setValidation(`请填写大于 0 且不超过 ${percentage(maximum)} 的让利百分点，最多 16 位小数。`); return; }
    if (!Decimal.safeParse(budget).success || (budget.split(".")[1]?.length || 0) > 18 || (enabled && compareDecimal(budget, "0") <= 0)) { setValidation("启用活动时，请填写大于 0 的赠送预算 ATG，最多 18 位小数。"); return; }
    if (!Number.isSafeInteger(Number(perCustomer)) || Number(perCustomer) < 1 || !Number.isSafeInteger(Number(total)) || Number(total) < 0) { setValidation("使用次数须为有效整数，每位客户至少可使用一次。"); return; }
    if (!agree || !plan || new Date(ends).getTime() <= new Date(starts).getTime()) { setValidation("请核对套餐、活动起止时间并确认活动条款。"); return; }
    const result = await command.run({ configuration: { ...(item ? { campaign_id: item.id } : {}), package_id: plan, concession_rate: percentage(points, true), gift_atg: "0", eligibility, starts_at: new Date(starts).toISOString(), ends_at: new Date(ends).toISOString(), per_customer_limit: Number(perCustomer), total_use_limit: Number(total), gift_budget_atg: budget, discount_budget_usd: "0", enabled, expected_version: item?.version || 0, evidence: "customer explicitly confirmed KOL promotion terms" } });
    if (result) close();
  }}><h2 className="text-lg font-semibold">{item ? "修订推广活动" : "创建推广活动"}</h2>
    <fieldset disabled={command.busy || command.unresolved} className="grid gap-5 sm:grid-cols-2">
      <label className="text-sm">适用套餐<select aria-label="适用套餐" value={plan} onChange={event => setPlan(event.target.value)} required className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3">{value?.package_id && !plans.some(current => current.id === value.package_id) && <option value={String(value.package_id)}>原套餐（当前未开放购买）</option>}{plans.map(plan => <option key={plan.id} value={plan.id}>{plan.name}</option>)}</select></label>
      <label className="text-sm">本人让利百分点<Input className="mt-2" value={points} onChange={event => setPoints(event.target.value)} inputMode="decimal" placeholder={`最高 ${percentage(maximum)}`} required /><span className="mt-2 block text-xs text-ink-mute">例如填写 10，表示让出直接佣金的 10 个百分点。</span></label>
      <label className="text-sm">活动开始<Input className="mt-2" type="datetime-local" value={starts} onChange={event => setStarts(event.target.value)} required /></label>
      <label className="text-sm">活动结束<Input className="mt-2" type="datetime-local" value={ends} onChange={event => setEnds(event.target.value)} required /></label>
      <label className="text-sm">适用客户<select aria-label="适用客户" value={eligibility} onChange={event => setEligibility(event.target.value)} className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3"><option value="any">所有直属受邀客户</option><option value="first_order">首次购买客户</option><option value="existing">已有购买记录客户</option></select></label>
      <label className="text-sm">每位客户最多使用次数<Input className="mt-2" type="number" min="1" step="1" value={perCustomer} onChange={event => setPerCustomer(event.target.value)} required /></label>
      <label className="text-sm">活动总使用次数上限<Input className="mt-2" type="number" min="0" step="1" value={total} onChange={event => setTotal(event.target.value)} required /><span className="mt-2 block text-xs text-ink-mute">0 表示不单独限制总次数。</span></label>
      <label className="text-sm">活动赠送预算 ATG<Input className="mt-2" inputMode="decimal" value={budget} onChange={event => setBudget(event.target.value)} required /><span className="mt-2 block text-xs text-ink-mute">本活动额外赠送总量的上限；启用时须大于 0，用完后不再受理新订单。</span></label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />启用活动</label>
      <label className="flex items-start gap-2 text-sm leading-6 sm:col-span-2"><input type="checkbox" checked={agree} onChange={event => setAgree(event.target.checked)} className="mt-1" />我确认活动让利、期限和使用上限，发布后仅对有效直属受邀客户生效。</label>
    </fieldset>
    {validation && <p role="alert" className="text-sm text-danger">{validation}</p>}{!!command.error && <ErrorPanel error={command.error} />}{blocked && <Notice>{blocked}</Notice>}
    <div className="flex gap-3"><Button type="submit" disabled={command.busy || (!command.unresolved && !agree)}>{command.busy ? "正在发布…" : command.unresolved ? "重试原活动发布" : "确认发布活动"}</Button><Button type="button" variant="outline" disabled={command.busy || command.unresolved} onClick={close}>取消编辑</Button></div>
  </form></Card>;
}
