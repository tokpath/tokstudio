"use client";
import { localizeNumberText } from "@/lib/i18n";
import { useI18n } from "@/components/i18n";
import { useState } from "react";
import Link from "next/link";
import { Gift, Share2 } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { collection, CommandResultSchema, PlanSchema, ReferralSchema, ResourcePageSchema, type Entity } from "@/lib/contracts";
import { compareDecimal } from "@/lib/decimal";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { CopyButton, useCustomerPendingGuard } from "./customer-ui";
import { FieldHelp } from "./field-help";
import { activityShareURL, concessionPreview, percentage } from "./customer-discovery";
import { AlertText, Empty, ErrorPanel, Heading, Loading, Money, Notice, Status, When, useCommand } from "./shared";

const localTime = (date: Date) => new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

export function CustomerActivities() {
  const { t, locale } = useI18n();
  const rows = useQuery({ queryKey: ["customer-activities"], queryFn: () => request("/api/v1/me/activities/workspace", ResourcePageSchema) });
  const referrals = useQuery({ queryKey: ["referrals"], queryFn: () => request("/api/v1/me/referrals", ReferralSchema) });
  const plans = useQuery({ queryKey: ["purchase-plans"], queryFn: () => request("/api/v1/me/plans/catalog", collection(PlanSchema)) });
  const [editing, setEditing] = useState<Entity | null | undefined>(), [published, setPublished] = useState<{ id: string; plan: string }>();
  const qualified = referrals.data?.qualified && referrals.data.function_status === "active";
  return <><Heading title={t("我的 KOL 活动")} description={t("从本人的直接佣金比例中让利，为直属受邀客户提供套餐加赠。")} actions={<Button asChild variant="outline"><Link href="/workspace/referrals">{t("邀请与收益")}</Link></Button>} />
    {referrals.isPending || rows.isPending || plans.isPending ? <Loading /> : (referrals.isError && !referrals.data) || (rows.isError && !rows.data) || (plans.isError && !plans.data) ? <ErrorPanel error={referrals.error || rows.error || plans.error} retry={() => { void referrals.refetch(); void rows.refetch(); void plans.refetch(); }} /> : <div className="space-y-5">
      {!qualified && <Notice>{t("当前尚未取得可用的 KOL 推广资格。可以在邀请与收益页查看资格金额及功能状态。")}</Notice>}
      {published && <Card className="border-success/40"><h2 className="font-semibold text-success">{t("活动已发布，可以分享给直属受邀客户。")}</h2><ActivityShare inviteURL={referrals.data.invite_url} planID={published.plan} campaignID={published.id} /></Card>}
      {qualified && editing === undefined && <Button onClick={() => setEditing(null)} disabled={!plans.data.items.length}>{t("创建推广活动")}</Button>}
      {qualified && editing !== undefined && <ActivityForm key={editing?.id || "new"} item={editing} plans={plans.data.items} maximum={referrals.data.direct_rate} close={() => setEditing(undefined)} published={(id, plan) => { setPublished({ id, plan }); setEditing(undefined); }} />}
      {!rows.data.items.length ? <Empty title={t("还没有推广活动")}>{t("取得有效资格后，选择套餐、让利比例及活动期限即可发布。")}</Empty> : rows.data.items.map(item => <Card key={item.id}><div className="flex flex-wrap items-start justify-between gap-4"><div><div className="mb-3"><Status value={item.status || (item.values.enabled ? "enabled" : "disabled")} /></div><h2 className="font-semibold">{String(item.values.name || plans.data.items.find(plan => plan.id === item.values.package_id)?.name || t("历史套餐活动"))}</h2><p className="mt-3 text-sm text-ink-secondary">{t("让利 {points} 个百分点", { points: localizeNumberText(percentage(String(item.values.concession_rate)), locale) })}</p><p className="mt-3 text-xs text-ink-mute"><When value={String(item.values.starts_at)} /> {t("至")} <When value={String(item.values.ends_at)} /></p><p className="mt-3 text-xs text-ink-mute">{t("每位客户最多 {count} 次", { count: String(item.values.per_customer_limit) })} · {t("总次数")} {item.values.total_use_limit ? t("最多 {count} 次", { count: String(item.values.total_use_limit) }) : t("不单独限制")}</p><p className="mt-3 text-xs text-ink-mute">{({ any: t("所有直属受邀客户"), first_order: t("首次购买客户"), existing: t("已有购买记录客户") } as Record<string, string>)[String(item.values.eligibility)]}</p><p className="mt-3 text-xs text-ink-mute">{t("已用 {used}，待付款占用 {reserved}", { used: String(item.values.used_uses || 0), reserved: String(item.values.reserved_uses || 0) })}</p></div>{qualified && editing === undefined && <Button variant="outline" onClick={() => setEditing(item)}>{t("修订活动")}</Button>}</div><ActivityShare inviteURL={referrals.data.invite_url} planID={String(item.values.package_id)} campaignID={item.id} /></Card>)}
      <div className="flex items-center gap-2 text-sm text-ink-secondary"><span>{t("活动让利与组合规则")}</span><FieldHelp title={t("活动让利与组合规则")}><p>{t("可让利上限为本人当前直接佣金比例。额外赠送按套餐原始购买额度和套餐自带赠送之和计算，经营方活动加赠不再计入计算基数。")}</p><p>{t("活动仅对有效直属受邀客户生效。是否可以叠加经营方活动，由经营方明确开放，以客户购买报价为准。修订只影响后续订单，已成交的权益和比例保留。")}</p><p>{t("单笔最多使用一个 KOL 活动，不能跨 KOL 领取；后续购买与续费不会自动继承。")}</p></FieldHelp></div>
    </div>}
  </>;
}

export function ActivityForm({ item, plans, maximum, close, published }: { item: Entity | null; plans: ReturnType<typeof PlanSchema.parse>[]; maximum: string; close: () => void; published?: (id: string, plan: string) => void }) {
  const { t, locale } = useI18n();
  const value = item?.values;
  const [name, setName] = useState(String(value?.name || ""));
  const [plan, setPlan] = useState(String(value?.package_id || plans[0]?.id || ""));
  const [points, setPoints] = useState(value ? percentage(String(value.concession_rate)) : "");
  const [eligibility, setEligibility] = useState(String(value?.eligibility || "any"));
  const [starts, setStarts] = useState(localTime(value ? new Date(String(value.starts_at)) : new Date()));
  const [ends, setEnds] = useState(localTime(value ? new Date(String(value.ends_at)) : new Date(Date.now() + 7 * 86400000)));
  const [perCustomer, setPerCustomer] = useState(String(value?.per_customer_limit || 1));
  const [total, setTotal] = useState(String(value?.total_use_limit || 0));
  const [enabled, setEnabled] = useState(value ? !!value.enabled : true), [agree, setAgree] = useState(false), [validation, setValidation] = useState<string>();
  const command = useCommand("/api/v1/me/activities", CommandResultSchema);
  const blocked = useCustomerPendingGuard(command.busy || command.unresolved);
  const selectedPlan = plans.find(item => item.id === plan);
  const preview = selectedPlan ? concessionPreview(selectedPlan.purchase_atg, selectedPlan.gift_atg, points, maximum) : undefined;
  return <Card><form className="space-y-5" onSubmit={async event => {
    event.preventDefault();
    if (command.unresolved) { const result = await command.run(undefined); if (result) { if (published) published(result.id, plan); else close(); } return; }
    setValidation(undefined);
    if (!/^(0|[1-9]\d*)(\.\d{1,16})?$/.test(points) || compareDecimal(percentage(points, true), "0") <= 0 || compareDecimal(percentage(points, true), maximum) > 0) { setValidation("请填写大于 0 且不超过 {maximum} 的让利百分点，最多 16 位小数。"); return; }
    if (!Number.isSafeInteger(Number(perCustomer)) || Number(perCustomer) < 1 || !Number.isSafeInteger(Number(total)) || Number(total) < 0) { setValidation("使用次数须为有效整数，每位客户至少可使用一次。"); return; }
    if (!agree || !plan || !Number.isFinite(new Date(starts).getTime()) || !Number.isFinite(new Date(ends).getTime()) || new Date(ends).getTime() <= new Date(starts).getTime()) { setValidation("请核对套餐、活动起止时间并确认活动条款。"); return; }
    const result = await command.run({ configuration: { ...(item ? { campaign_id: item.id } : {}), name: name.trim(), package_id: plan, concession_rate: percentage(points, true), eligibility, starts_at: new Date(starts).toISOString(), ends_at: new Date(ends).toISOString(), per_customer_limit: Number(perCustomer), total_use_limit: Number(total), enabled, expected_version: item?.version || 0, evidence: "customer explicitly confirmed KOL promotion terms" } });
    if (result) { if (published) published(result.id, plan); else close(); }
  }}><h2 className="text-lg font-semibold">{item ? t("修订推广活动") : t("创建推广活动")}</h2>
    <div className="grid items-start gap-6 lg:grid-cols-[1.4fr_1fr]"><fieldset disabled={command.busy || command.unresolved} className="grid gap-5 sm:grid-cols-2">
      <label className="text-sm sm:col-span-2">{t("活动名称（可选）")}<Input aria-label={t("活动名称（可选）")} className="mt-2" value={name} onChange={event => setName(event.target.value)} maxLength={100} placeholder={selectedPlan?.name} /></label>
      <label className="text-sm">{t("适用套餐")}<select aria-label={t("适用套餐")} value={plan} onChange={event => setPlan(event.target.value)} required className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3">{value?.package_id && !plans.some(current => current.id === value.package_id) && <option value={String(value.package_id)}>{t("原套餐（当前未开放购买）")}</option>}{plans.map(plan => <option key={plan.id} value={plan.id}>{plan.name}</option>)}</select></label>
      <div className="text-sm"><span className="inline-flex items-center">{t("本人让利百分点")}<FieldHelp title={t("本人让利百分点")}><p>{t("例如填写 10，表示让出直接佣金的 10 个百分点。")}</p><p>{t("可让利上限为本人当前直接佣金比例。额外赠送按套餐原始购买额度和套餐自带赠送之和计算，经营方活动加赠不再计入计算基数。")}</p></FieldHelp></span><Input aria-label={t("本人让利百分点")} className="mt-2" value={points} onChange={event => setPoints(event.target.value)} inputMode="decimal" placeholder={t("最高 {maximum}", { maximum: localizeNumberText(percentage(maximum), locale) })} required /></div>
      <label className="text-sm">{t("活动开始")}<Input className="mt-2" type="datetime-local" value={starts} onChange={event => setStarts(event.target.value)} required /></label>
      <label className="text-sm">{t("活动结束")}<Input className="mt-2" type="datetime-local" value={ends} onChange={event => setEnds(event.target.value)} required /></label>
      <label className="text-sm">{t("适用客户")}<select aria-label={t("适用客户")} value={eligibility} onChange={event => setEligibility(event.target.value)} className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3"><option value="any">{t("所有直属受邀客户")}</option><option value="first_order">{t("首次购买客户")}</option><option value="existing">{t("已有购买记录客户")}</option></select></label>
      <label className="text-sm">{t("每位客户最多使用次数")}<Input className="mt-2" type="number" min="1" step="1" value={perCustomer} onChange={event => setPerCustomer(event.target.value)} required /></label>
      <div className="text-sm"><span className="inline-flex items-center">{t("活动总使用次数上限")}<FieldHelp title={t("活动总使用次数上限")}><p>{t("0 表示不单独限制总次数。")}</p></FieldHelp></span><Input aria-label={t("活动总使用次数上限")} className="mt-2" type="number" min="0" step="1" value={total} onChange={event => setTotal(event.target.value)} required /></div>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />{t("启用活动")}</label>
      <label className="flex items-start gap-2 text-sm leading-6 sm:col-span-2"><input type="checkbox" checked={agree} onChange={event => setAgree(event.target.checked)} className="mt-1" />{t("我确认活动让利、期限和使用上限，发布后仅对有效直属受邀客户生效。")}</label>
    </fieldset><aside aria-live="polite" aria-label={t("活动效果预览")} className="rounded-card border border-brand/30 bg-brand-soft/15 p-5 lg:sticky lg:top-6"><div className="mb-4 flex items-center gap-2"><Gift className="h-4 w-4 text-brand-emphasis" /><h3 className="font-semibold">{t("活动效果预览")}</h3></div>{preview && selectedPlan ? <><p className="text-sm font-medium">{selectedPlan.name}</p><dl className="mt-4 space-y-4 text-sm"><div><dt className="mb-1 text-xs text-ink-mute">{t("套餐基础总额度")}</dt><dd><Money value={preview.base} /></dd></div><div><dt className="mb-1 text-xs text-ink-mute">{t("本活动每单加赠")}</dt><dd className="text-xl font-semibold text-brand-emphasis">+<Money value={preview.gift} /></dd></div><div><dt className="mb-1 text-xs text-ink-mute">{t("含本活动的服务额度")}</dt><dd><Money value={preview.total} /></dd></div><div className="border-t border-hairline pt-4"><dt className="mb-1 text-xs text-ink-mute">{t("本人剩余直接佣金比例")}</dt><dd>{localizeNumberText(preview.remainingPoints, locale)}%</dd></div></dl><p className="mt-4 break-words font-mono text-xs leading-6 text-ink-mute">{localizeNumberText(preview.base, locale)} ATG × {localizeNumberText(points, locale)}% = {localizeNumberText(preview.gift, locale)} ATG</p></> : <p className="text-sm leading-7 text-ink-mute">{t("选择套餐并填写有效的让利百分点，立即查看加赠与剩余佣金。")}</p>}<p className="mt-4 text-xs leading-6 text-ink-mute">{t("仅为活动预览，最终权益、次数与组合资格由购买报价核实。")}</p></aside></div>
    {validation && <AlertText>{t(validation, { maximum: localizeNumberText(percentage(maximum), locale) })}</AlertText>}{!!command.error && <ErrorPanel error={command.error} />}{blocked && <Notice>{blocked}</Notice>}{command.unresolved && <Notice>{t("操作结果待核实，配置已锁定，再次提交会重试原请求。")}</Notice>}
    <div className="flex gap-3"><Button type="submit" disabled={command.busy || (!command.unresolved && !agree)}>{command.busy ? t("正在发布…") : command.unresolved ? t("重试原活动发布") : t("确认发布活动")}</Button><Button type="button" variant="outline" disabled={command.busy || command.unresolved} onClick={close}>{t("取消编辑")}</Button></div>
  </form></Card>;
}

function ActivityShare({ inviteURL, planID, campaignID }: { inviteURL: string; planID: string; campaignID: string }) {
  const { t } = useI18n();
  const url = activityShareURL(inviteURL, planID, campaignID);
  return url ? <div className="mt-5 border-t border-hairline pt-4"><div className="mb-2 flex items-center gap-2 text-xs text-ink-mute"><Share2 className="h-3.5 w-3.5" /><span>{t("活动分享链接")}</span><FieldHelp title={t("活动分享链接")}><p>{t("链接保留你的原邀请入口。已有客户的经营方和邀请人不会改变；活动只向有效直属受邀客户开放。")}</p><p>{t("单笔最多使用一个 KOL 活动，不能跨 KOL 领取；后续购买与续费不会自动继承。")}</p></FieldHelp></div><p className="mb-3 break-all text-xs leading-6">{url}</p><CopyButton value={url} label={t("复制活动链接")} /></div> : null;
}
