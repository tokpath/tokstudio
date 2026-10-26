"use client";
import { useI18n } from "@/components/i18n";

import Link from "next/link";
import { useEffect, useState, type ComponentType, type ReactNode } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { ArrowRight, Check, Circle, ClipboardCheck, ShieldCheck, Sparkles } from "lucide-react";
import { request } from "@/lib/api-client";
import { EntitySchema, OperationSchema, ResourcePageSchema, SignedDecimal, type Entity, type JSONValue, type ResourcePage } from "@/lib/contracts";
import { canReadResource, resourceLabel } from "@/lib/navigation";
import { useProduct } from "./context";
import { ResourceCollection, ResourceScreen, OperatorPanel, OperatorPage, ResourceValue, ReadFacts } from "./operator-resources";
import { Heading, Empty, ErrorPanel, Loading, Money, Notice, Stat, Status, When } from "./shared";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { subjectURL, subjectReturnURL } from "./operator-list-state";
import { FieldHelp } from "./field-help";

export function OperatorWorkflowScreen({ operatorID, resource, record }: { operatorID: string; resource: string; record?: string }) {
  const { t } = useI18n();
  const [subject, setSubject] = useState<{ resource: "customers" | "orders"; row: Entity } | undefined>(() => record && (resource === "customers" || resource === "orders") ? { resource, row: { id: record, values: {} } } : undefined);
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const workflow = workflows[resource];
  useEffect(() => { setSubject(record && (resource === "customers" || resource === "orders") ? { resource, row: { id: record, values: {} } } : undefined); }, [record, resource]);
  useEffect(() => { const restore = () => { const id = new URLSearchParams(window.location.search).get("record"); setSubject(id && (resource === "customers" || resource === "orders") ? { resource, row: { id, values: {} } } : undefined); }; window.addEventListener("popstate", restore); return () => window.removeEventListener("popstate", restore); }, [resource]);
  if (!canReadResource(workspace, resource)) return <Empty title={t("没有查看权限")}>{t("此页面需要当前经营方明确授予相应权限。")}</Empty>;
  const close = () => { const target = subjectReturnURL(operatorID, resource); if (target.split("?")[0] === window.location.pathname && !new URL(target, window.location.origin).searchParams.has("record")) { window.history.pushState(window.history.state, "", target); setSubject(undefined); } else { window.location.assign(target); } };
  return <>
    <div hidden={!!subject}>
      {workflow && <><Heading eyebrow={t(workflow.eyebrow)} title={t(resourceLabel(resource))} description={t(workflow.description)} />{!["plans","pricing","catalog","providers","routes"].includes(resource) && <WorkflowGuide operatorID={operatorID} resource={resource} />}</>}
      <ResourceScreen key={`${operatorID}:${resource}`} operatorID={operatorID} resource={resource} record={resource === "customers" || resource === "orders" ? undefined : record} embedded={!!workflow} layout={cardResources.has(resource) ? "cards" : "table"} inspectionLinks onInspect={(name, row) => { if (name === "customers" || name === "orders") { const returnTo = window.location.pathname + window.location.search; window.history.pushState(window.history.state, "", subjectURL(operatorID, name, row.id, returnTo)); setSubject({ resource: name, row }); } }} />
    </div>
    {subject && <OperatorSubjectContent key={`${subject.resource}:${subject.row.id}`} operatorID={operatorID} resource={subject.resource} row={subject.row} close={close} fullPage />}
  </>;
}

export function OperatorOverview({ operatorID }: { operatorID: string }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const isPlatform = workspace?.operator.kind === "platform";
  const permissions = workspace?.permissions || [];
  const focus = permissions.includes("*") || permissions.includes("business.manage") || permissions.includes("staff.manage") ? "owner" : permissions.some(permission => ["payments.manage", "refunds.manage", "withdrawals.manage", "invoices.manage"].includes(permission)) ? "finance" : permissions.some(permission => ["platform.manage", "catalog.manage", "providers.manage"].includes(permission)) ? "technology" : permissions.includes("support.manage") ? "support" : "operations";
  const resources = ["overview", "business-status", "alerts", "approvals", "orders", "refunds", "withdrawals"];
  const sources = useQueries({ queries: resources.map((resource, index) => ({ queryKey: ["operator-home", operatorID, resource], queryFn: ({ signal }: { signal: AbortSignal }) => request(`/api/v1/operators/${encodeURIComponent(operatorID)}/${resource}`, ResourcePageSchema, { signal }), enabled: canReadResource(workspace, resource) && (index < 4 || ["owner", "finance", "support"].includes(focus)) })) });
  const overview = sources[0], business = sources[1], risks = sources[2], approvals = sources[3];
  const businessTasks = [
    { resource: "orders", source: sources[4], title: "跟进订单收款与交付", detail: "从原订单核对收款、分配和交付状态。", rows: (sources[4].data?.items || []).filter(row => ["pending", "partial", "paid", "exception", "pending_payment", "partially_paid", "paid_pending_delivery"].includes(row.status || "")) },
    { resource: "refunds", source: sources[5], title: "跟进未结退款事项", detail: "先核对审批与原退款结果，再按已授权动作办理。", rows: (sources[5].data?.items || []).filter(row => row.values.record_kind === "case" && !["completed", "rejected", "cancelled", "canceled"].includes(row.status || "")) },
    { resource: "withdrawals", source: sources[6], title: "跟进佣金提现", detail: "独立复核与实际付款分别核实，未知结果继续查询原操作。", rows: (sources[6].data?.items || []).filter(row => !["paid", "paid_out", "completed", "rejected", "cancelled", "canceled", "closed"].includes(row.status || "")) },
  ].filter(task => canReadResource(workspace, task.resource) && ["owner", "finance", "support"].includes(focus) && task.rows.length > 0);
  const metrics: Record<string, string> = Object.fromEntries((overview.data?.items || []).flatMap(item => typeof item.values.metric === "string" && SignedDecimal.safeParse(item.values.value).success ? [[item.values.metric, item.values.value as string]] : []));
  const metricVisible = (metric: typeof metricDefinitions[number]) => focus === "finance" ? ["receipt-valuations", "payments", "refunds"].includes(metric.resource) : focus === "technology" ? metric.resource === "requests" : focus !== "support";
  const applicableMetrics = metricDefinitions.filter(metric => metricVisible(metric) && (!isPlatform || !upstreamMetricKeys.has(metric.key)));
  const outstanding = applicableMetrics.filter(metric => metric.task && metrics[metric.key] && !/^0(?:\.0+)?$/.test(metrics[metric.key]));
  const unknownTasks = overview.data && applicableMetrics.some(metric => metric.task && metrics[metric.key] === undefined);
  const activeRisks = (risks.data?.items || []).filter(row => !["normal", "resolved", "cleared"].includes(row.status || "") && (!isPlatform || !upstreamMetricKeys.has(String(row.values.metric))));
  const businessRow = business.data?.items[0];
  const isOpening = businessRow && ["registration", "new_purchase", "model_usage"].every(key => businessRow.values[key] === false);
  const focusTitles: Record<string, string> = { owner: "经营总览与待办", finance: "今天需要核实的资金事项", technology: "今天需要处理的服务异常", support: "今天需要跟进的客户事项", operations: "今天需要处理的经营事项" };
  const focusResources: Record<string, string[]> = { owner: ["customers", "plans", "payments", isPlatform ? "providers" : "procurement"], finance: ["payments", "receipt-valuations", "refunds", "reconciliation", "withdrawals", "invoices"], technology: ["requests", "providers", "routes", "jobs"], support: ["customers", "orders", "refunds", "restrictions"], operations: ["customers", "plans", "marketing", "coverage", "alerts", isPlatform ? "providers" : "procurement"] };
  const destination = (resource: string) => `/manage/${encodeURIComponent(operatorID)}/${resource}`;
  return <>
    <Heading eyebrow={t("经营工作台")} title={t(focusTitles[focus])} description={t("{operator}的待办、营业项目和服务责任。所有数据均来自当前空间已授权的经营范围。", { operator: workspace?.operator.name || t("当前经营方") })} actions={<span className="rounded-full border border-hairline bg-canvas-raised px-3 py-2 text-xs text-ink-mute">{workspace?.operator.kind === "platform" ? t("平台经营") : workspace?.operator.kind === "oem" ? t("OEM 独立经营") : t("渠道独立经营")}
    </span>} />
    {overview.isLoading && <Loading />}{overview.isError && <div className="mb-5">
      <ErrorPanel error={overview.error} retry={() => void overview.refetch()} />
    </div>}
    {isOpening && <OpeningChecklist operatorID={operatorID} row={businessRow} />}
    {isPlatform && <details className="mb-5"><summary className="cursor-pointer text-sm text-brand-emphasis">{t("平台直接维护供应商与上游路由")}</summary><div className="mt-3"><Notice><p>{t("平台没有上级采购账户。模型履约依赖供应商接入、有效成本价和合格上游路由；平台继续承担客户服务与对下供货责任。")}</p><div className="mt-3 flex flex-wrap gap-4">{[["providers", t("维护模型供应商")], ["routes", t("配置路由")]].filter(([resource]) => canReadResource(workspace, resource)).map(([resource, label]) => <Link key={resource} className="text-brand-emphasis underline" href={destination(resource)}>{t(label)}</Link>)}</div></Notice></div></details>}
    <div className="mb-5 flex flex-wrap gap-2" aria-label={t("常用办理入口")}>{focusResources[focus].filter(resource => canReadResource(workspace, resource)).map(resource => <Button key={resource} variant="ghost" size="sm" asChild><Link href={destination(resource)}>{t(resourceLabel(resource))}<ArrowRight size={14} className="ml-2" /></Link></Button>)}</div>
    <div className="grid items-start gap-6 xl:grid-cols-[1.15fr_1fr]">
      <Card className="min-w-0">
        <div className="mb-5 flex items-center gap-3">
          <ClipboardCheck size={19} className="text-brand-emphasis" />
          <h2 className="text-lg font-semibold">{t("待办事项")}</h2>
        </div>
        <div className="space-y-3">{businessTasks.map(task => <TaskRow key={task.resource} title={task.title} description={t(task.detail)} count={t("{count} 条已读取", { count: task.rows.length })} href={destination(task.resource)} />)}{outstanding.map(metric => <TaskRow key={metric.key} title={metric.task!} description={t(metric.detail)} count={t("{count} 笔", { count: metrics[metric.key] })} href={canReadResource(workspace, metric.resource) ? destination(metric.resource) : undefined} />)}{approvals.data && approvals.data.items.length > 0 && <TaskRow title={t("完成独立复核")} description={t("到原业务核对来源、条款和证据，再由具备权限的人员办理。")} count={t("{count} 条已读取", { count: approvals.data.items.length })} href={destination("approvals")} />}{!businessTasks.length && !outstanding.length && !(approvals.data?.items.length) && <p className="py-6 text-sm leading-7 text-ink-mute">{unknownTasks ? t("部分待办指标尚未核实，请到相应业务页继续查询。") : overview.data || approvals.data ? t("已读取范围内暂无未决待办。其他业务仍可从对应板块查询。") : t("当前权限未提供经营待办数据。可从下方已授权入口继续工作。")}
        </p>}
        </div>{sources.slice(4).map((source, index) => source.isError && <div key={resources[index + 4]} className="mt-4"><ErrorPanel error={source.error} retry={() => void source.refetch()} compact /></div>)}{approvals.isError && <div className="mt-4">
          <ErrorPanel error={approvals.error} retry={() => void approvals.refetch()} compact />
        </div>}
      </Card>
      <Card className="min-w-0">
        <div className="mb-5 flex items-center gap-3">
          <ShieldCheck size={19} className="text-brand-emphasis" />
          <h2 className="text-lg font-semibold">{t("服务风险")}</h2>
        </div>{canReadResource(workspace, "alerts") ? risks.isLoading ? <Loading /> : risks.isError ? <ErrorPanel error={risks.error} retry={() => void risks.refetch()} compact /> : activeRisks.length ? <div className="space-y-4">{activeRisks.slice(0, 5).map(row => <div key={row.id} className="border-b border-hairline pb-4 last:border-0 last:pb-0">
          <div className="flex justify-between gap-3">
            <p className="text-sm font-medium">{t(metricDefinitions.find(metric => metric.key === row.values.metric)?.label || "经营风险预警")}
            </p>{row.status && <Status value={row.status} />}
          </div>
          <p className="mt-2 text-xs text-ink-mute">{t("当前值")} <ResourceValue value={row.values.value} /> {t("· 检查于")} <When value={typeof row.values.observed_at === "string" ? row.values.observed_at : undefined} />
          </p>
        </div>)}
          <Link className="inline-flex items-center gap-2 text-sm text-brand-emphasis" href={destination("alerts")}>{t("查看风险与处置")}<ArrowRight size={14} />
          </Link>
        </div> : <p className="py-4 text-sm leading-7 text-ink-mute">{isPlatform ? t("已读取的适用风险规则暂无异常。平台继续核实供应商健康、路由容量和调用成本。") : t("已读取的风险规则暂无异常。模型调用仍须具备足额采购余额和合格路由。")}</p> : <p className="py-4 text-sm leading-7 text-ink-mute">{t("风险详情需要当前空间的运营权限。")}</p>}
      </Card>
    </div>
    {overview.data && applicableMetrics.some(metric => metric.unit) && <div className="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{applicableMetrics.filter(metric => metric.unit).map(metric => <div key={metric.key}><Stat label={t(metric.label)} value={metrics[metric.key] ?? t("尚未核实")} unit={metrics[metric.key] !== undefined ? metric.unit : undefined} detail={metrics[metric.key] !== undefined ? undefined : t("尚未取得可核实的指标，请到履约与超卖或采购页面查询原始责任。")} /><FieldHelp title={t(metric.label)}>{t(metric.detail)}</FieldHelp></div>)}
    </div>}
    {overview.data && applicableMetrics.filter(metric => !metric.unit && metrics[metric.key] !== undefined).length > 0 && <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{applicableMetrics.filter(metric => !metric.unit && metrics[metric.key] !== undefined).map(metric => <div key={metric.key} className="rounded-control border border-hairline px-4 py-3">
      <p className="text-xs text-ink-mute">{t(metric.label)}
      </p>
      <p className="mt-2 text-lg font-semibold">{metrics[metric.key]} <span className="text-xs font-normal text-ink-mute">{t("笔")}</span>
      </p>
    </div>)}
    </div>}
    <Card className="mt-6">
      <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-3">
            <Sparkles size={19} className="text-brand-emphasis" />
            <h2 className="text-lg font-semibold">{t("营业与开业检查")}</h2>
          </div>
          <p className="mt-2 text-sm leading-6 text-ink-mute">{t("逐项开放注册、销售、调用和结算。暂停新增业务后，存量权益和资金责任继续办理。")}</p>
        </div>{canReadResource(workspace, "business-status") && <Button asChild size="sm" variant="outline">
          <Link href={destination("business-status")}>{t("检查并调整营业项目")}</Link>
        </Button>}
      </div>{business.isLoading ? <Loading /> : business.isError ? <ErrorPanel error={business.error} retry={() => void business.refetch()} compact /> : businessRow ? <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">{businessFeatures.map(([key, label, help]) => <div key={key} className="rounded-xl bg-canvas p-4">
        <div className="flex items-center gap-2">{businessRow.values[key] ? <Check size={16} className="text-success" /> : <Circle size={15} className="text-ink-mute" />}
          <h3 className="text-sm font-medium">{t(label)}
          </h3>
          <span className="ml-auto text-xs text-ink-mute">{businessRow.values[key] ? t("已开放") : t("未开放")}
          </span>
        </div>
        <p className="mt-2 text-xs leading-6 text-ink-mute">{isPlatform && key === "model_usage" ? t("公开模型、有效成本价与合格上游路由须就绪") : t(help)}
        </p>
      </div>)}
      </div> : <p className="text-sm text-ink-mute">{t("营业项目详情需业务管理权限。")}</p>}
    </Card>


  </>;
}

const ReadinessSchema = z.array(z.object({ feature: z.string(), requirements: z.array(z.object({ code: z.string(), description: z.string(), ready: z.boolean(), resource: z.string().optional() })) }));
function OpeningChecklist({ operatorID, row }: { operatorID: string; row: Entity }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const checks = ReadinessSchema.safeParse(row.values.readiness);
  const missing = [...new Map((checks.success ? checks.data.filter(check => row.values[check.feature] !== true).flatMap(check => check.requirements.filter(requirement => !requirement.ready)) : []).map(requirement => [requirement.code, requirement])).values()];
  const resources: Record<string, string> = { incentives_policy: "commissions", verification_mail: "mail", reward_coverage: "coverage", package: "plans", payment: "payment-methods", catalog_pricing: "pricing", coverage: "coverage" };
  return <Card className="mb-6 border-brand/30"><div className="mb-4 flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-lg font-semibold">{t("开业前，先完成这些准备")}</h2><p className="mt-2 text-sm text-ink-secondary">{t("按真实检查结果补齐配置，再逐项开放营业。")}</p></div><Button asChild variant="outline"><Link href={`/manage/${encodeURIComponent(operatorID)}/business-status`}>{t("进入营业设置")}</Link></Button></div>{missing.length > 0 ? <ul className="divide-y divide-hairline">{missing.map(requirement => { const resource = requirement.resource || resources[requirement.code]; return <li key={requirement.code} className="flex flex-wrap items-center justify-between gap-3 py-3"><span className="text-sm">{t(requirement.description)}</span>{resource && canReadResource(workspace, resource) ? <Link className="inline-flex items-center gap-1 text-sm text-brand-emphasis" href={`/manage/${encodeURIComponent(operatorID)}/${resource}`}>{t("去完成")}<ArrowRight size={14} /></Link> : <span className="text-xs text-ink-mute">{t("请由具备相应权限的人员办理。")}</span>}</li>; })}</ul> : <p className="text-sm leading-7 text-ink-secondary">{t(checks.success ? "已读取的配置检查已通过，请到营业设置选择要开放的项目。" : "配置检查尚未返回，请到营业设置核实具体依赖。")}</p>}</Card>;
}

const SubjectContextSchema = z.object({ subject: EntitySchema, sections: z.array(z.object({ resource: z.string(), title: z.string(), page: ResourcePageSchema })), next_actions: z.array(z.string()).optional(), subject_operations: z.array(OperationSchema).optional() });
export function OperatorSubjectPanel(props: { operatorID: string; resource: "customers" | "orders"; row: Entity; close: () => void }) { return <OperatorSubjectContent {...props} />; }
function OperatorSubjectContent({ operatorID, resource, row, close, fullPage = false }: { operatorID: string; resource: "customers" | "orders"; row: Entity; close: () => void; fullPage?: boolean }) {
  const { t } = useI18n();
  const [active, setActive] = useState(""), [cursor, setCursor] = useState(""), [history, setHistory] = useState<string[]>([]);
  const [nestedOrder, setNestedOrder] = useState<Entity>();
  const [blocked, setBlocked] = useState(false);
  const endpoint = `/api/v1/operators/${encodeURIComponent(operatorID)}/${resource === "customers" ? "customer-context" : "order-context"}/${encodeURIComponent(row.id)}`;
  const data = useQuery({ queryKey: ["operator-subject", endpoint], queryFn: ({ signal }) => request(endpoint, SubjectContextSchema, { signal }) });
  const params = new URLSearchParams({ section: active, cursor });
  const paged = useQuery({ queryKey: ["operator-subject-page", endpoint, active, cursor], queryFn: ({ signal }) => request(`${endpoint}?${params}`, SubjectContextSchema, { signal }), enabled: !!active && !!cursor });
  const subject = data.data?.subject || row;
  const sections = data.data?.sections || [];
  const currentSection = sections.find(item => item.resource === active) || sections[0];
  const section = cursor ? paged.data?.sections.find(item => item.resource === active) || currentSection : currentSection;
  const profile = resource === "customers";
  const title = profile ? String(subject.values.name || subject.values.display_name || t("客户工作台")) : t("订单与收款办理");
  const Container = (fullPage ? OperatorPage : OperatorPanel) as ComponentType<{ open?: boolean; title: string; description: string; close: () => void; onBlockedChange: (blocked: boolean) => void; children: ReactNode }>;
  return <Container open title={title} description={t("{subject} {id} · 仅查看当前经营范围内明确授权的关联记录", { subject: t(profile ? "客户" : "订单"), id: row.id })} close={close} onBlockedChange={setBlocked}>
    {data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <>
      <Card className="mb-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="font-semibold">{profile ? String(subject.values.email || t("客户资料")) : t(orderKindLabel(subject.values.kind))}
            </h3>{profile && <p className="mt-2 text-xs text-ink-mute">{t("注册于")} <When value={typeof subject.values.registered_at === "string" ? subject.values.registered_at : undefined} />
            </p>}
          </div>{subject.status && <Status value={subject.status} />}
        </div>{!profile && <>
          <div className="mt-5 grid gap-4 sm:grid-cols-3">
            <AmountFact label={t("订单应付")} value={subject.values.due_usd} />
            <AmountFact label={t("外部款已分配")} value={subject.values.external_allocated_usd} />
            <AmountFact label={t("USD 余额支付")} value={subject.values.internal_usd} />
            <AmountFact label={t("佣金支付")} value={subject.values.commission_usd} />
            <AmountFact label={t("购买权益")} value={subject.values.paid_atg} unit="ATG" />
            <div>
              <p className="mb-1 text-xs text-ink-mute">{t("实际交付")}</p>
              <When value={typeof subject.values.completed_at === "string" ? subject.values.completed_at : undefined} />
            </div>
          </div>
          <OrderProgress subject={subject} />{!!data.data?.subject_operations?.length && <div className="mt-5">
            <ResourceCollection operatorID={operatorID} resource="orders" page={{ title: t("订单办理"), description: "", items: [subject], columns: [], operations: [], row_operations: data.data.subject_operations }} embedded actionsOnly />
          </div>}</>}
        <details className="mt-5 border-t border-hairline pt-4">
          <summary className="cursor-pointer text-sm text-brand-emphasis">{profile ? t("客户归属与保存资料") : t("成交条款与原始订单")}
          </summary>
          <div className="mt-4">
            <ReadFacts value={subject.values} />
          </div>
        </details>
      </Card>
      {!!data.data?.next_actions?.length && <div className="mb-6">
        <Notice>
          <p className="font-medium text-ink">{t("下一步办理")}</p>
          <ul className="mt-2 list-disc space-y-1 pl-4">{data.data.next_actions.map((action, index) => <li key={index}>{t(action)}
          </li>)}
          </ul>
        </Notice>
      </div>}
      {sections.length ? <>
        <div className="mb-5 flex flex-wrap gap-2" role="tablist" aria-label={profile ? t("客户关联业务") : t("订单关联业务")}>{sections.map(item => <button key={item.resource} type="button" role="tab" disabled={blocked} aria-selected={section?.resource === item.resource} onClick={() => { setActive(item.resource); setCursor(""); setHistory([]); }} className={`rounded-control border px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-60 ${section?.resource === item.resource ? "border-brand bg-brand-soft text-brand-emphasis" : "border-hairline bg-canvas-raised text-ink-secondary"}`}>{t(item.title)}
        </button>)}
        </div>{section && <div role="tabpanel" aria-label={t(section.title)}>
          <div className="mb-4">
            <h3 className="font-semibold">{t(section.title)}
            </h3>
            <p className="mt-1 text-xs leading-6 text-ink-mute">{t(section.page.description)}
            </p>
          </div>{cursor && paged.isPending ? <Loading /> : cursor && paged.isError ? <ErrorPanel error={paged.error} retry={() => void paged.refetch()} /> : <ResourceCollection key={`${row.id}:${section.resource}`} page={section.page} operatorID={operatorID} resource={section.resource} embedded initialValues={resource === "orders" ? { order_id: row.id } : undefined} inspectionLinks={fullPage} onInspect={fullPage ? undefined : (name, item) => { if (name === "orders" && resource === "customers") setNestedOrder(item); }} />}
          <div className="mt-5 flex justify-end gap-2">{!!history.length && <Button variant="outline" size="sm" disabled={blocked} onClick={() => { const previous = [...history]; setCursor(previous.pop() || ""); setHistory(previous); }}>{t("上一页关联记录")}</Button>}{section.page.next_cursor && <Button variant="outline" size="sm" disabled={blocked} onClick={() => { setActive(section.resource); setHistory(previous => [...previous, cursor]); setCursor(section.page.next_cursor!); }}>{t("下一页关联记录")}</Button>}
          </div>
        </div>}</> : <Empty title={t("暂无可查看的关联业务")}>{t("当前授权范围内没有其他业务记录。")}</Empty>}
    </>}
    {nestedOrder && <OperatorSubjectPanel operatorID={operatorID} resource="orders" row={nestedOrder} close={() => setNestedOrder(undefined)} />}
  </Container>;
}

function AmountFact({ label, value, unit = "USD" }: { label: string; value?: JSONValue; unit?: string }) {
  const { t } = useI18n();
  return <div>
    <p className="mb-1 text-xs text-ink-mute">{t(label)}
    </p>{typeof value === "string" ? <Money value={value} unit={unit} /> : <span className="text-ink-mute">—</span>}
  </div>;
}
function OrderProgress({ subject }: { subject: Entity }) {
  const { t } = useI18n();
  const complete = subject.status === "completed" || subject.status === "fulfilled";
  return <div className="mt-5 grid gap-3 rounded-control bg-canvas p-4 sm:grid-cols-3">
    <div>
      <p className="text-xs text-ink-mute">{t("订单")}</p>
      <p className="mt-1 text-sm">{complete ? t("成交已交付") : t("保留原成交条款")}
      </p>
    </div>
    <div>
      <p className="text-xs text-ink-mute">{t("款项")}</p>
      <p className="mt-1 text-sm">{t("收款、分配与手续费独立核实")}</p>
    </div>
    <div>
      <p className="text-xs text-ink-mute">{t("交付")}</p>
      <p className="mt-1 text-sm">{complete ? t("查看已发放权益与来源") : subject.status === "paid_pending_delivery" || subject.status === "paid" ? t("资金足额，检查覆盖后交付") : t("以原订单资金状态为准")}
      </p>
    </div>
  </div>;
}
function orderKindLabel(kind?: JSONValue) { return ({ package: "套餐购买订单", topup: "USD 余额充值订单", renewal: "套餐续费订单", package_purchase: "套餐购买订单", package_renewal: "套餐续费订单", customer_usd_topup: "USD 余额充值订单" } as Record<string, string>)[String(kind)] || "原始订单"; }
function TaskRow({ title, description, count, href }: { title: string; description: string; count: string; href?: string }) {
  const { t } = useI18n();
  const body = <>
    <div className="flex items-center justify-between gap-3">
      <span className="text-sm font-medium">{t(title)}
      </span>
      <span className="rounded-full bg-brand-soft px-2 py-1 text-xs text-brand-emphasis">{count}
      </span>
    </div>
    <p className="mt-2 text-xs leading-6 text-ink-mute">{t(description)}
    </p>{!href && <p className="mt-1 text-xs text-ink-mute">{t("请由具备相应权限的人员办理。")}</p>}</>; return href ? <Link className="block rounded-control bg-canvas px-4 py-4 transition-colors hover:bg-brand-soft" href={href}>{body}
    </Link> : <div className="rounded-control bg-canvas p-4">{body}
    </div>;
}
function WorkflowGuide({ operatorID, resource }: { operatorID: string; resource: string }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const flow = workflows[resource];
  return <details className="th-help-inline"><summary className="cursor-pointer text-sm text-brand-emphasis">{t("办理说明与相关入口")}</summary><div className="mt-4">
    <ol className="grid gap-5 md:grid-cols-3">{flow.steps.map(([title, text], index) => <li key={title} className="flex gap-3">
      <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-soft text-xs font-semibold text-brand-emphasis">{index + 1}
      </span>
      <div>
        <h2 className="text-sm font-medium">{t(title)}
        </h2>
        <p className="mt-1 text-xs leading-6 text-ink-mute">{t(text)}
        </p>
      </div>
    </li>)}
    </ol>{flow.links.some(link => canReadResource(workspace, link)) && <div className="mt-5 flex flex-wrap items-center gap-2 border-t border-hairline pt-4">
      <span className="mr-1 text-xs text-ink-mute">{t("相关办理")}</span>{flow.links.filter(link => canReadResource(workspace, link)).map(link => <Button asChild variant="ghost" size="sm" key={link}>
        <Link href={`/manage/${encodeURIComponent(operatorID)}/${link}`}>{t(resourceLabel(link))}
          <ArrowRight size={12} className="ml-1" aria-hidden />
        </Link>
      </Button>)}
    </div>}
  </div></details>;
}

const businessFeatures = [["registration", "新客户注册", "正式验证后建立归属与邀请关系"], ["new_purchase", "套餐新购", "商品、收款与销售承载能力须就绪"], ["renewal", "套餐续费", "按客户确认和当前成交条款办理"], ["model_usage", "模型调用", "授权、价格、路由和采购余额须就绪"], ["activity", "活动参与", "规则、预算与赠送履约能力须就绪"], ["settlement", "资金结算", "继续处理存量退款、佣金与资金责任"]] as const;
const upstreamMetricKeys = new Set(["sales_space_atg", "procurement_available_atg", "oversell_used_atg"]);
const metricDefinitions: { key: string; label: string; unit?: string; detail: string; task?: string; resource: string }[] = [
  { key: "sales_space_atg", label: "剩余销售承载空间", unit: "ATG", detail: "已扣除承诺及待交付责任，用于判断能否新增销售。", resource: "coverage" },
  { key: "procurement_available_atg", label: "可用采购余额", unit: "ATG", detail: "用于真实模型调用；不提供采购授信。", resource: "procurement" },
  { key: "oversell_used_atg", label: "已使用超卖空间", unit: "ATG", detail: "只影响销售承载，不代表可以欠款调用。", resource: "coverage" },
  { key: "pending_receipt_facts", label: "实际款待核实与入账", detail: "先核实已收款原事实，避免让客户重复付款。", task: "处理实际款入账", resource: "receipt-valuations" },
  { key: "awaiting_valuation_receipts", label: "实际款待估值", detail: "按实际付款时点的独立估值证据办理。", task: "核实原币收款估值", resource: "receipt-valuations" },
  { key: "pending_usage", label: "用量待核实请求", detail: "先查询原请求结果，再核实占用与结算。", task: "排查未决模型请求", resource: "requests" },
  { key: "unverified_costs", label: "供应成本待核实", detail: "保留未决成本，不将未知金额计作零成本。", task: "核实供应商成本", resource: "requests" },
];
const cardResources = new Set(["plans", "marketing", "business-status", "payment-methods"]);
type Workflow = { eyebrow: string; title: string; description: string; steps: [string, string][]; links: string[] };
const workflows: Record<string, Workflow> = {
  customers: { eyebrow: "客户服务", title: "围绕一个客户，处理完整服务", description: "按姓名、邮箱或编号找到本经营方客户，进入客户工作台查看订单、资产和已授权的服务记录。", steps: [["找到客户", "核对邮箱、注册时间与当前归属。"], ["查看业务上下文", "客户工作台集中展示精确关联的订单、资产和售后。"], ["在原业务办理", "保留收款、交付、退款与推广记录各自的状态。"]], links: ["orders", "gifts", "restrictions"] },
  orders: { eyebrow: "资金办理 · 订单", title: "从成交到交付，一笔订单办到底", description: "收款事实、资金分配和权益发放分别核实。打开订单查看关联收款、付款证据与下一步办理。", steps: [["核对成交", "保留套餐版本、应付金额和购买/赠送条款。"], ["核实资金", "核实真实到账，按原订单分配，差额与费用独立记录。"], ["完成交付", "资金足额且覆盖满足后发放；异常沿用原订单重试。"]], links: ["payments", "receipt-valuations", "refunds"] },
  payments: { eyebrow: "资金办理 · 收款", title: "核实真实到账，再分配至原订单", description: "每笔收款保留实际币种、付款时点、估值和银行或通道证据。查看详情核对原始事实，按需办理分配。", steps: [["确认收款事实", "先核查外部唯一流水、收款账号和原币金额。"], ["核实本次估值", "尚未确定 USD 价值的真实款，先到估值待办核实。"], ["分配并追踪交付", "选取原订单及可分配金额，随后查看订单交付。"]], links: ["orders", "receipt-valuations", "evidence", "reconciliation"] },
  plans: { eyebrow: "商品与定价", title: "把套餐条款做成清楚的商品", description: "设置套餐总售价，分别记录购买额度与赠送额度。修改后新订单使用新条件，已有成交保留原条款。", steps: [["定义商品", "确认成交 USD、购买/赠送 ATG、有效期和公开条款。"], ["准备销售", "核对收款就绪、模型价格和销售承载空间。"], ["发布与续费", "分别控制新购与续费；涨价或权益减少时，由客户重新确认。"]], links: ["pricing", "payment-methods", "coverage", "marketing"] },
  pricing: { eyebrow: "商品与定价", title: "管理上架模型与各层调用价", description: "模型调用价格与采购额度的购买报价分别管理。新价格适用于新受理请求，已有请求保留受理快照。", steps: [["核对模型授权", "在已授权范围选择上架，平台停用后不能继续受理。"], ["设置有效价格", "设置客户或直属下级的 ATG 单价，也可按倍率跟随上级供货价。"], ["通知并检查覆盖", "系统自动处理调价通知、生效安排和承载重算。"]], links: ["coverage", "procurement", "plans"] },
  marketing: { eyebrow: "营销与推广", title: "让活动预算、资格与交付一起生效", description: "活动的折扣、额外赠送、参与对象和次数都按成交校验。经营方活动与 KOL 活动须明确配对后才可组合。", steps: [["选定套餐与对象", "使用已发布套餐，明确时间窗口和参与资格。"], ["设置预算与限次", "核对赠送履约、折扣预算及每位客户/全场次数。"], ["追踪推广效果", "保留活动版本与历史资格；暂停后续分佣独立办理。"]], links: ["plans", "commissions", "gifts", "coverage"] },
  commissions: { eyebrow: "营销与推广", title: "公布推荐规则，保留每笔佣金依据", description: "客户达到终身资格门槛后按两跳关系分佣。资格累计、佣金应计、冻结解冻与提现分别记录。", steps: [["公布资格规则", "确定净消费门槛、两跳比例和确认等待。"], ["管理后续资格", "审核风险不抹去历史合法佣金，暂停与终止分别处理。"], ["办理真实付款", "佣金可用后按提现规则申请、复核和核实付款。"]], links: ["customers", "marketing", "withdrawals"] },
  procurement: { eyebrow: "采购与合作", title: "采购补货与对下供货分开办理", description: "选择我方采购或我方供货视图，沿报价、付款和交付追踪。采购余额不过期，补货不锁定未来模型调用价。", steps: [["确认固定报价", "核对供货关系、购买/赠送采购 ATG 和退出条款。"], ["先全款到账", "真实支付或已核实人工收款；建单不增加余额。"], ["交付与有限补货", "完成库存交付；自动补货须有独立授权和限额。"]], links: ["operators", "coverage", "procurement-invoices", "exits"] },
  coverage: { eyebrow: "采购与合作", title: "看清销售责任，提前安排补货", description: "采购余额、已承诺服务与待交付责任共同影响销售承载。超卖授权用于销售管理，真实调用必须采购余额充足。", steps: [["核对当前责任", "查看购买与赠送服务所需的实际采购覆盖。"], ["设置授权和缓冲", "按明确权限设置销售超卖与预警缓冲。"], ["采购与风险处置", "承载不足时补货或限制新增销售，保留存量责任。"]], links: ["procurement", "alerts", "business-status"] },
  operators: { eyebrow: "采购与合作", title: "开通合作主体，明确各自经营责任", description: "平台可创建 OEM 和直属渠道，OEM 可创建自己的直属渠道。渠道没有下级经营方。", steps: [["确定主体与品牌", "明确实际经营主体、品牌和直接供货关系。"], ["邀请负责人", "合作主体负责人在自己的空间配置人员和收款。"], ["定制并开业", "上级按授权定制域名及外观，开通不自动开放营业。"]], links: ["contracts", "domains", "channel-themes", "takeovers"] },
  "business-status": { eyebrow: "设置 · 营业", title: "按真实依赖逐项开放营业", description: "先保存配置，再逐项执行开业检查。不同支付方式独立就绪，暂停新业务不消除存量权益和结算责任。", steps: [["准备必要配置", "核对商品、收款、模型授权、价格和采购余额。"], ["逐项检查开放", "分别控制注册、新购、续费、调用、活动与结算。"], ["持续处理存量", "遇到风险按项目暂停，继续处理退款、佣金和账单。"]], links: ["payment-methods", "plans", "pricing", "coverage"] },
  providers: { eyebrow: "平台技术", title: "登记供应商和供应商模型", description: "先登记供应商密钥，再添加供应商模型和能力。Bifrost 参考价目仅用于成本核算，对客 ATG 价格在商品与定价设置。", steps: [["管理密钥", "填写名称和密钥。修改已有密钥时，密钥留空表示不更换。"], ["添加供应商模型", "选择密钥、协议和模型名称，勾选能力，按需选择参考成本模型。"], ["再到路由配置绑定", "把这些供应商模型绑到公开模型，并选择选路策略。"]], links: ["catalog", "routes", "pricing"] },
  catalog: { eyebrow: "平台技术", title: "登记公开模型", description: "公开模型标识就是客户请求里使用的名字。这里填写标识并勾选能力。是否开放在列表里切换。", steps: [["填写标识", "填写客户请求里使用的公开模型标识。"], ["勾选能力", "勾选对话、图片、语音或视频。"], ["在列表里开放", "配好路由后，回到列表打开开关。没有路由时开关是灰的，打不开。"]], links: ["providers", "routes", "pricing"] },
  routes: { eyebrow: "平台技术", title: "把供应商模型绑到公开模型", description: "一个公开模型可以使用多个供应商模型。按列表顺序、最低延迟或预估最低成本选路，故障暂停与恢复由系统自动处理。", steps: [["选择公开模型", "选择客户请求里使用的那个模型。"], ["绑定供应商模型", "选择支持相应能力的供应商模型，上下移动调整优先顺序。"], ["选择策略", "固定优先级、已测到的最低延迟，或按最大用量估算的最低成本。"]], links: ["providers", "catalog", "pricing", "requests"] },
};
