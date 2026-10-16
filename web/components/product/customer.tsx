"use client";
import Link from "next/link";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { collection, CreditsSchema, OrderSchema, RequestSchema, ReferralSchema, CommissionSchema, NotificationSchema, CommandResultSchema } from "@/lib/contracts";
import { safeAssetURL } from "@/lib/decimal";
import { useProduct } from "./context";
import { WorkspaceShell } from "./workspace-shell";
import { CustomerKeys } from "./customer-keys";
import { CustomerPlans, PaymentAction } from "./customer-plans";
import { ResourceScreen } from "./management";
import { NotificationPreferences } from "./notification-preferences";
import { Playground } from "./playground";
import { Empty, ErrorPanel, Heading, Loading, Money, Notice, Stat, Status, Table, When, useCommand } from "./shared";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function CustomerPage({ section, initialPlan }: { section: string; initialPlan?: string }) {
  let content: React.ReactNode;
  switch (section) {
    case "": content = <Overview />; break;
    case "plans": content = <CustomerPlans initialPlan={initialPlan} />; break;
    case "keys": content = <CustomerKeys />; break;
    case "orders": content = <Orders />; break;
    case "requests": content = <Requests />; break;
    case "playground": content = <Playground />; break;
    case "referrals": content = <Referrals />; break;
    case "commissions": content = <Commissions />; break;
    case "notifications": content = <Notifications />; break;
    case "activities": case "withdrawals": case "refunds": case "invoices": case "account": case "topups": case "credit-sources": content = <ResourceScreen resource={`${section}/workspace`} customer />; break;
    default: content = <Empty title="页面不存在"><Link href="/workspace" className="underline">返回资产总览</Link></Empty>;
  }
  return <WorkspaceShell>{content}</WorkspaceShell>;
}
function Overview() {
  const { session } = useProduct();
  const credits = useQuery({ queryKey: ["credits"], queryFn: () => request("/api/v1/me/credits", CreditsSchema) });
  if (credits.isPending) return <Loading />;
  if (credits.isError) return <ErrorPanel error={credits.error} retry={() => void credits.refetch()} />;
  const value = credits.data;
  return <><Heading title={session?.kind === "customer" ? `${session.customer.display_name}，管理你的模型服务` : "资产总览"} description="可用余额、在途预占和已发生消费分别记录。所有 Key 共享本账号有效 ATG，资产归属于当前经营方。" actions={<Button asChild><Link href="/workspace/plans">购买套餐</Link></Button>} />
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3"><Stat label="可用服务额度" value={value.customer_atg.available} unit="ATG" detail="用于当前已授权并上架的模型" /><Stat label="请求预占" value={value.customer_atg.held} unit="ATG" detail="预占不是最终扣费，结算后释放差额" /><Stat label="可用支付余额" value={value.usd_balance.available} unit="USD" detail={`另有占用 ${value.usd_balance.held} USD`} /><Stat label="可用佣金" value={value.commission_usd.available} unit="USD" detail={`已占用 ${value.commission_usd.held} USD；来源限制 ${value.commission_usd.source_restricted||"0"} USD，限制部分不可新花或提现`} /><Stat label="待确认冻结佣金" value={value.commission_usd.frozen} unit="USD" detail="消费确认且无相关争议后按规则解冻" /><Stat label="佣金待抵扣" value={value.commission_usd.recovery_due} unit="USD" detail="仅按原错误佣金处理，不动用正常支付余额与 ATG" /></div>
    <div className="mt-7 grid gap-5 lg:grid-cols-2"><Card><h2 className="mb-4 text-lg font-semibold">服务额度来源</h2><p className="text-sm leading-7">购买部分剩余 <Money value={value.customer_atg.purchased_remaining} /><br />赠送部分剩余 <Money value={value.customer_atg.gift_remaining} /></p><p className="my-4 text-xs leading-6 text-ink-mute">更早到期的有效来源优先，同一套餐订单内购买和各项赠送按比例使用。到期后释放的未用预占转入到期记录。</p><Link className="text-sm text-brand-emphasis underline" href="/workspace/credit-sources">查看来源与到期记录</Link></Card><Card><h2 className="mb-4 text-lg font-semibold">推广资格</h2><p className="text-sm leading-8">已确认真实支出 <Money value={value.qualification.confirmed_usd} unit="USD" /><br />待确认 <Money value={value.qualification.pending_usd} unit="USD" /><br />待确认冲正 <Money value={value.qualification.pending_correction_usd||"0"} unit="USD" /><br />当前门槛 <Money value={value.qualification.threshold_usd} unit="USD" /></p><p className="my-4 text-xs text-ink-mute">{value.qualification.qualified_at ? <>资格取得于 <When value={value.qualification.qualified_at} /></> : "待确认金额不计资格，到期购买价值计资格但不产生佣金。"}</p><Link href="/workspace/referrals" className="text-sm text-brand-emphasis underline">查看邀请与 KOL 状态</Link></Card></div>
    <div className="mt-7 flex flex-wrap gap-3"><Button asChild variant="outline"><Link href="/workspace/topups">充值 USD 支付余额</Link></Button><Button asChild variant="outline"><Link href="/workspace/refunds">办理未用 USD 退款</Link></Button><Button asChild variant="outline"><Link href="/workspace/withdrawals">申请佣金提现</Link></Button></div>
  </>;
}
function Orders() {
  const paging = useCursor();
  const [selected, setSelected] = useState<string>();
  const data = useQuery({ queryKey: ["orders", paging.cursor, paging.search], queryFn: () => request(paging.path("/api/v1/me/orders"), collection(OrderSchema)), refetchInterval: query => query.state.data?.items.some(item => ["pending_payment", "partially_paid", "paid", "paid_pending_delivery", "pending_verification"].includes(item.status)) ? 5000 : false });
  const order = data.data?.items.find(item => item.id === selected);
  return <><Heading title="购买记录" description="收款、成交和资产交付各有独立状态。少付、真实多付、迟付及已收款未交付均会保留待处理记录。" actions={<Button variant="outline" onClick={() => void data.refetch()}>刷新状态</Button>} /><RecordSearch paging={paging} />{order && <Card className="mb-6"><div className="flex justify-between"><h2 className="text-lg font-semibold">订单详情</h2><Button variant="ghost" size="sm" onClick={() => setSelected(undefined)}>关闭</Button></div><PaymentAction order={order} /></Card>}{data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <Table rows={data.data.items} getID={item => item.id} empty="购买或充值订单将在这里展示。" columns={[{ label: "订单", cell: item => <div><p>{item.plan_name || item.kind}</p><p className="mt-2 break-all font-mono text-xs text-ink-mute">{item.id}</p><p className="mt-2 text-xs">经营方：{item.operator_name}</p></div> }, { label: "应付与已核实", cell: item => <div><Money value={item.amount_usd} unit="USD" /><p className="mt-2 text-xs">已核实 <Money value={item.paid_usd} unit="USD" /></p></div> }, { label: "状态", cell: item => <Status value={item.status} /> }, { label: "创建时间", cell: item => <When value={item.created_at} /> }, { label: "操作", cell: item => <Button variant="outline" size="sm" onClick={() => setSelected(item.id)}>查看详情与支付</Button> }]} />}<CursorControls paging={paging} next={data.data?.next_cursor} /></>;
}
function Requests() {
  const paging = useCursor();
  const [selected, setSelected] = useState<string>();
  const data = useQuery({ queryKey: ["requests", paging.cursor, paging.search], queryFn: () => request(paging.path("/api/v1/me/requests"), collection(RequestSchema)) });
  const detail = useQuery({ queryKey: ["request-detail", selected], queryFn: () => request(`/api/v1/me/requests/${encodeURIComponent(selected || "")}`, RequestSchema), enabled: !!selected });
  return <><Heading title="用量与账单" description="按稳定请求编号核对模型、Key、价格和来源。重复查询与取得同一任务结果不会新增模型调用费。" /><RecordSearch paging={paging} />{selected && <Card className="mb-6"><div className="mb-4 flex justify-between"><h2 className="text-lg font-semibold">请求明细</h2><Button variant="ghost" size="sm" onClick={() => setSelected(undefined)}>关闭</Button></div>{detail.isPending ? <Loading /> : detail.isError ? <ErrorPanel error={detail.error} retry={() => void detail.refetch()} /> : <div className="space-y-5"><p className="break-all font-mono text-xs">{detail.data.id}</p><Status value={detail.data.status} />{detail.data.reason && <Notice>{detail.data.reason}</Notice>}<p>实际扣费 <Money value={detail.data.net_charged_atg ?? detail.data.charged_atg} /> · 在途预占 <Money value={detail.data.held_atg} /></p>{detail.data.actual_atg&&<p className="text-sm text-ink-secondary">本次真实用量计价 <Money value={detail.data.actual_atg}/> · 经营方承担的额度缺口 <Money value={detail.data.merchant_shortfall_atg||"0"}/></p>}{detail.data.corrected_atg !== undefined && <div className="space-y-2 text-sm"><p>原始计价 <Money value={detail.data.actual_atg || "0"}/> · 原始实扣 <Money value={detail.data.charged_atg}/></p><p>已纠正用量计价 <Money value={detail.data.corrected_atg}/> · 已返还实扣 <Money value={detail.data.corrected_charged_atg || "0"}/></p><p>纠正后净计价 <Money value={detail.data.net_actual_atg || detail.data.actual_atg || "0"}/> · 净实扣 <Money value={detail.data.net_charged_atg ?? detail.data.charged_atg}/></p><p className="text-ink-mute">以下计量和来源保留原始受理与扣减事实，纠正记录不会覆盖原事件。</p></div>}{detail.data.components && <Table rows={detail.data.components} getID={item => item.component} columns={[{ label: "计量组成", cell: item => item.component }, { label: "用量", cell: item => `${item.quantity} ${item.unit}` }, { label: "受理时单价", cell: item => item.price_description || (item.price_atg ? <Money value={item.price_atg} /> : "—") }, { label: "金额", cell: item => <Money value={item.amount_atg} /> }]} />}{detail.data.allocations && <Table rows={detail.data.allocations} getID={item => `${item.source_name}:${item.kind}`} columns={[{ label: "来源", cell: item => item.source_name }, { label: "类型", cell: item => <div>{item.kind}{item.sources?.map((source,i)=><p key={i} className="mt-2 text-xs">{source.kind} <Money value={source.amount_atg}/></p>)}</div> }, { label: "扣减", cell: item => <Money value={item.amount_atg} /> }, { label: "有偿基数", cell: item => <Money value={item.paid_basis_usd} unit="USD" /> }]} />}{safeAssetURL(detail.data.result_url) && <div><a className="text-sm text-brand-emphasis underline" href={safeAssetURL(detail.data.result_url)} rel="noreferrer">取得暂存结果</a><p className="mt-2 text-xs text-ink-mute">暂存到期：<When value={detail.data.result_expires_at} /></p></div>}</div>}</Card>}{data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <Table rows={data.data.items} getID={item => item.id} empty="尚无模型调用。可创建 Key 或进入模型体验。" columns={[{ label: "模型与 Key", cell: item => <div><p>{item.model_id}</p><p className="mt-2 text-xs text-ink-mute">{item.key_name}</p></div> }, { label: "状态", cell: item => <Status value={item.status} /> }, { label: "费用", cell: item => <div><Money value={item.net_charged_atg ?? item.charged_atg} /><p className="mt-2 text-xs">预占 <Money value={item.held_atg} /></p></div> }, { label: "受理时间", cell: item => <When value={item.created_at} /> }, { label: "明细", cell: item => <Button size="sm" variant="outline" onClick={() => setSelected(item.id)}>查看来源与结果</Button> }]} />}<CursorControls paging={paging} next={data.data?.next_cursor} /></>;
}
function Referrals() {
  const [copied, setCopied] = useState(false), [copyError, setCopyError] = useState(false);
  const data = useQuery({ queryKey: ["referrals"], queryFn: () => request("/api/v1/me/referrals", ReferralSchema) });
  return <><Heading title="邀请与 KOL 资格" description="邀请关系限定同一经营方，只计算直接、间接两跳。取得 KOL 资格后，按下级实际有偿调用和订单快照形成佣金。" />{data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <><Card><div className="flex flex-wrap items-start justify-between gap-4"><div><p className="text-xs text-ink-mute">你的邀请入口</p><p className="mt-3 break-all text-sm">{data.data.invite_url}</p><p className="mt-3 font-mono text-sm">邀请码：{data.data.invite_code}</p></div><Button onClick={async () => { try { await navigator.clipboard.writeText(data.data.invite_url); setCopied(true); } catch { setCopyError(true); } }}>{copied ? "已复制" : "复制邀请链接"}</Button></div>{copyError && <p className="mt-3 text-sm text-danger">复制失败，请手动选中复制。</p>}</Card><div className="my-5 grid gap-4 sm:grid-cols-2 xl:grid-cols-3"><Stat label="直接邀请客户" value={String(data.data.direct_count)} /><Stat label="间接邀请客户" value={String(data.data.indirect_count)} /><Stat label="已确认真实支出" value={data.data.confirmed_usd} unit="USD" /><Stat label="待确认金额" value={data.data.pending_usd} unit="USD" /><Stat label="当前资格门槛" value={data.data.threshold_usd} unit="USD" /><Stat label="分佣状态" value={data.data.qualified ? "已取得 KOL 资格" : "尚未取得 KOL 资格"} detail={data.data.function_status} /></div><Notice>购买时固定直接比例 {data.data.direct_rate}、间接比例 {data.data.indirect_rate}；消费时分别核实两跳邀请人的有效资格。赠送、到期金额和经营方承担的额度缺口不产生佣金。下级客户的余额、Key 和请求内容不会向推广者开放。</Notice>{data.data.qualified && <Button asChild className="mt-5"><Link href="/workspace/activities">管理 KOL 推广活动</Link></Button>}</>}</>;
}
function Commissions() {
  const paging = useCursor();
  const data = useQuery({ queryKey: ["commissions", paging.cursor, paging.search], queryFn: () => request(paging.path("/api/v1/me/commissions"), collection(CommissionSchema)) });
  return <><Heading title="佣金明细" description="冻结、可用、占用与待抵扣分别记录。正确佣金不因后续规则调整或善意补偿而追溯修改。" actions={<Button asChild variant="outline"><Link href="/workspace/withdrawals">提现与付款记录</Link></Button>} /><RecordSearch paging={paging} />{data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <Table rows={data.data.items} getID={item => item.id} empty="符合资格的两跳有偿调用将形成佣金记录。" columns={[{ label: "来源", cell: item => <div><p>{item.description || item.type}</p><p className="mt-2 text-xs text-ink-mute">{item.hop === 1 ? "直接邀请" : "间接邀请"} · {item.source_id}</p></div> }, { label: "有偿基数", cell: item => <Money value={item.basis_usd} unit="USD" /> }, { label: "佣金", cell: item => <Money value={item.amount_usd} unit="USD" /> }, { label: "状态", cell: item => <Status value={item.status} /> }, { label: "发生与确认", cell: item => <div><When value={item.created_at} /><p className="mt-2 text-xs text-ink-mute">应确认 <When value={item.confirms_at} /></p></div> }]} />}<CursorControls paging={paging} next={data.data?.next_cursor} /></>;
}
function Notifications() {
  const paging = useCursor();
  const data = useQuery({ queryKey: ["notifications", paging.cursor, paging.search], queryFn: () => request(paging.path("/api/v1/me/notifications"), collection(NotificationSchema)) });
  return <><Heading title="通知" description="资金、资产、安全、履约与重要规则属于必要业务通知。营销退订不关闭这些通知；需要重新授权的事项会单独提供确认操作。" /><NotificationPreferences /><RecordSearch paging={paging} />{data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : !data.data.items.length ? <Empty title="暂无通知" /> : <div className="space-y-4">{data.data.items.map(item => <Notification key={item.id} item={item} />)}</div>}<CursorControls paging={paging} next={data.data?.next_cursor} /></>;
}
function Notification({ item }: { item: ReturnType<typeof NotificationSchema.parse> }) {
  const read = useCommand(`/api/v1/me/notifications/${item.id}/read`, CommandResultSchema);
  const href = safeAssetURL(item.action_url);
  return <Card><div className="flex justify-between gap-5"><h2 className="font-semibold">{!item.read_at && <span className="mr-2 inline-block h-2 w-2 rounded-full bg-brand" />}{item.title}</h2><span className="text-xs text-ink-mute">{item.necessary ? "必要业务通知" : "营销通知"}</span></div><p className="mt-4 whitespace-pre-wrap text-sm leading-7 text-ink-secondary">{item.body}</p><div className="mt-5 flex flex-wrap items-center gap-4 text-xs text-ink-mute"><When value={item.created_at} />{item.delivery_status && <span>邮件：{item.delivery_status}</span>}{href && <a href={href} className="text-brand-emphasis underline">查看相关事项</a>}{!item.read_at && <Button size="sm" variant="outline" disabled={read.busy} onClick={() => void read.run({})}>标为已读</Button>}</div>{!!read.error && <div className="mt-3"><ErrorPanel error={read.error} /></div>}</Card>;
}

function useCursor() {
  const [cursor, setCursor] = useState(""), [history, setHistory] = useState<string[]>([]), [search, setSearch] = useState("");
  return { cursor, history, search, setSearch: (value: string) => { setCursor(""); setHistory([]); setSearch(value); }, path: (base: string) => { const params = new URLSearchParams(); if (cursor) params.set("cursor", cursor); if (search) params.set("q", search); return `${base}${params.size ? `?${params}` : ""}`; }, next: (value: string) => { setHistory(previous => [...previous, cursor]); setCursor(value); }, previous: () => { const prior = [...history]; setCursor(prior.pop() || ""); setHistory(prior); } };
}
function RecordSearch({ paging }: { paging: ReturnType<typeof useCursor> }) { const [value, setValue] = useState(paging.search); return <form className="mb-5 flex gap-3" onSubmit={event => { event.preventDefault(); paging.setSearch(value); }}><Input aria-label="搜索明细" placeholder="搜索业务编号、模型或关联记录" className="max-w-lg" maxLength={200} value={value} onChange={event => setValue(event.target.value)} /><Button type="submit" variant="outline">查询</Button></form>; }
function CursorControls({ paging, next }: { paging: ReturnType<typeof useCursor>; next?: string | null }) { return <div className="mt-5 flex justify-end gap-3">{!!paging.history.length && <Button variant="outline" onClick={paging.previous}>上一页</Button>}{next && <Button variant="outline" onClick={() => paging.next(next)}>下一页</Button>}</div>; }
