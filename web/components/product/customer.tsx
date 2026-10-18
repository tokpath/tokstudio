"use client";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { collection, NotificationSchema, CommandResultSchema } from "@/lib/contracts";
import { safeAssetURL } from "@/lib/decimal";
import { WorkspaceShell } from "./workspace-shell";
import { CustomerKeys } from "./customer-keys";
import { CustomerPlans } from "./customer-plans";
import { CustomerModels } from "./customer-models";
import { CustomerOverview } from "./customer-overview";
import { CustomerOrders, CustomerRequests, CursorControls, RecordSearch, useCursor } from "./customer-records";
import { CustomerReferrals, CustomerCommissions } from "./customer-referrals";
import { CustomerActivities } from "./customer-activities";
import { ResourceScreen } from "./management";
import { NotificationPreferences } from "./notification-preferences";
import { Playground } from "./playground";
import { Empty, ErrorPanel, Heading, Loading, When, useCommand } from "./shared";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

export function CustomerPage({ section, initialPlan }: { section: string; initialPlan?: string }) {
  let content: React.ReactNode;
  switch (section) {
    case "": content = <CustomerOverview />; break;
    case "models": content = <CustomerModels />; break;
    case "plans": content = <CustomerPlans initialPlan={initialPlan} />; break;
    case "keys": content = <CustomerKeys />; break;
    case "orders": content = <CustomerOrders />; break;
    case "requests": content = <CustomerRequests />; break;
    case "playground": content = <Playground />; break;
    case "referrals": content = <CustomerReferrals />; break;
    case "commissions": content = <CustomerCommissions />; break;
    case "activities": content = <CustomerActivities />; break;
    case "notifications": content = <Notifications />; break;
    case "withdrawals": case "refunds": case "invoices": case "account": case "topups": case "credit-sources": content = <ResourceScreen resource={`${section}/workspace`} customer />; break;
    default: content = <Empty title="页面不存在"><Link href="/workspace" className="underline">返回概览</Link></Empty>;
  }
  return <WorkspaceShell>{content}</WorkspaceShell>;
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
