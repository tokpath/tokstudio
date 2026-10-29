"use client";
import { useI18n } from "@/components/i18n";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, ArrowUpRight, Code2, KeyRound, Layers3, ReceiptText, ShieldCheck } from "lucide-react";
import { request } from "@/lib/api-client";
import { collection, ModelSchema, PlanSchema, PublicContextSchema, type Plan } from "@/lib/contracts";
import { useProduct } from "./context";
import { Empty, ErrorPanel, Heading, Loading, Money, Notice } from "./shared";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ContextHelp, CopyButton } from "./customer-ui";
import { capabilityLabels, ModelCatalog } from "./customer-models";
import { planPeriods, totalCredits } from "./customer-finance";
import { FieldHelp } from "./field-help";

type CampaignLink = { plan?: string; activity?: string; invite?: string };
export function purchaseURL(planID: string, customer: boolean, campaign?: CampaignLink) {
  const query = new URLSearchParams({ plan: planID });
  if (campaign?.activity && campaign.plan === planID) query.set("kol_activity", campaign.activity);
  const path = `/workspace/plans?${query}`;
  if (customer) return path;
  const auth = new URLSearchParams({ next: path });
  if (campaign?.invite) auth.set("invite", campaign.invite);
  return `/${campaign?.invite ? "register" : "login"}?${auth}`;
}
function PublicPlanCard({ plan, campaign }: { plan: Plan; campaign?: CampaignLink }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const selected = campaign?.plan === plan.id;
  return <Card className={`flex min-w-0 flex-col ${selected ? "border-brand" : ""}`}>
    <div className="flex flex-wrap items-start justify-between gap-3"><h2 className="text-lg font-semibold">{plan.name}</h2>{selected && campaign.activity && <span className="rounded-full bg-brand-soft px-2.5 py-1 text-xs text-brand-emphasis">{t("分享链接中的套餐")}</span>}</div>
    <p className="th-plan-price"><Money value={plan.price_money} unit="MONEY" /></p>
    <div className="th-plan-allowance"><p className="mb-2 text-xs text-ink-secondary">{t("合计服务额度")}</p><p className="text-2xl font-semibold"><Money value={totalCredits(plan.purchase_atg, plan.gift_atg)} /></p><div className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs text-ink-secondary"><span>{t("购买额度")} <Money value={plan.purchase_atg} /></span><span>{t("套餐赠送")} <Money value={plan.gift_atg} /></span></div></div>
    <div className="my-5 flex flex-wrap items-center justify-between gap-2 text-sm"><span>{t(planPeriods[plan.validity])}</span><span className="text-xs text-ink-mute">{plan.auto_renew_available ? t("可单独授权到期续费") : t("单次购买")}</span></div>
    {plan.description && <details className="mb-5 border-t border-hairline pt-3 text-xs text-ink-secondary"><summary className="w-fit cursor-pointer py-1 font-medium">{t("服务条款")}</summary><p className="mt-2 whitespace-pre-line leading-7">{plan.description}</p></details>}
    <div className="mt-auto pt-1"><Button asChild className="w-full"><Link href={purchaseURL(plan.id, session?.kind === "customer", campaign)}>{selected && campaign.activity ? t("继续，核实活动资格") : t("选择套餐，核对成交条款")}<ArrowRight /></Link></Button>{selected && campaign.activity && session?.kind !== "customer" && <Link className="mt-3 block text-center text-xs text-brand-emphasis underline" href={`/login?next=${encodeURIComponent(purchaseURL(plan.id, true, campaign))}`}>{t("已有账号，登录核实活动")}</Link>}</div>
  </Card>;
}
export function PublicHome() {
  const { t } = useI18n();
  const { context, session } = useProduct();
  const models = useQuery({ queryKey: ["public-models"], queryFn: () => request("/api/v1/public/models", collection(ModelSchema)) });
  const plans = useQuery({ queryKey: ["public-plans"], queryFn: () => request("/api/v1/public/plans", collection(PlanSchema)) });
  return <main className="mx-auto max-w-[1360px] px-5 pb-16 pt-10 lg:px-8 lg:pt-12">
    <section className="th-hero">
      <div className="min-w-0"><p className="th-eyebrow mb-6 text-brand-emphasis">{context?.brand.name || "TokenHub"} / {t("MODEL SERVICES")}</p><h1>{t("一个品牌入口，")}<br /><span className="text-brand-emphasis">{t("连接你的模型与应用。")}</span></h1><p className="mt-6 max-w-xl text-base leading-8 text-ink-secondary">{t("选择模型，购买通用额度，用一个 API 入口接入你的应用。多个 Key 分别管理访问和预算，每笔消费都能找到依据。")}</p><div className="mt-8 flex flex-wrap gap-3"><Button asChild size="lg"><Link href="/models">{t("探索模型与价格")}<ArrowUpRight /></Link></Button><Button asChild size="lg" variant="outline"><Link href="/plans">{t("选择通用套餐")}</Link></Button></div><Link href={session?.kind === "customer" ? "/workspace/models" : "/register"} className="mt-6 inline-flex items-center gap-2 text-sm font-medium text-brand-emphasis">{session?.kind === "customer" ? t("接入模型") : t("创建账号，开始接入")}<ArrowRight className="size-4" /></Link></div>
      <div className="th-hero-console"><div className="th-console-content">
        <div className="flex items-center justify-between gap-3"><span className="th-eyebrow th-console-note">{t("YOUR MODEL WORKSPACE")}</span><ShieldCheck className="size-5 text-[#9dbdff]" /></div>
        <div className="th-console-endpoint"><div className="mb-4 flex items-center gap-2"><span className="rounded-md border border-[#4c6386] px-2 py-1 font-mono text-[10px] text-[#b8cfff]">API</span><span className="text-xs th-console-note">{t("品牌 API Base URL")}</span></div><code className="block break-all text-sm leading-7 text-[#edf4ff]">{context?.brand.api_base_url || t("品牌 API 入口待配置")}</code>{context?.brand.api_base_url && <div className="th-console-copy mt-4"><CopyButton value={context.brand.api_base_url} label={t("复制 Base URL")} /></div>}</div>
        <ol className="space-y-5">{[{ title: "选择模型与套餐", copy: "同一账号额度适用于当前经营方开放的模型。" }, { title: "创建应用 Key", copy: "保存一次性密钥，按需限制预算与模型。" }, { title: "复制代码，核对消费", copy: "在接入页获取示例，运行后查看请求账单。" }].map((item, index) => <li key={item.title} className="flex items-start gap-3"><span className="grid size-7 shrink-0 place-items-center rounded-lg border border-[#415777] font-mono text-xs text-[#b8cfff]">{index + 1}</span><div className="min-w-0"><p className="text-sm font-medium">{t(item.title)}</p><p className="mt-1 text-xs leading-6 th-console-note">{t(item.copy)}</p></div></li>)}</ol>
        <Link href="/workspace/models" className="mt-6 flex items-center justify-between gap-3 border-t border-[#3a4b65] pt-5 text-sm text-[#c7d9ff]"><span className="flex items-center gap-2"><Code2 className="size-4" />{t("查看接入指南与代码")}</span><ArrowUpRight className="size-4 shrink-0" /></Link>
      </div></div>
    </section>
    <section className="th-feature-row">{[{ icon: Layers3, title: "一个额度池，多种模型", copy: "购买与赠送保留各自来源和期限，多个有效套餐可以并存。" }, { icon: KeyRound, title: "为每个应用单独建 Key", copy: "分别设定预算、模型与并发，所有 Key 共享账号有效额度。" }, { icon: ReceiptText, title: "从请求追溯每笔费用", copy: "查看受理价格、实际用量、额度来源和净扣费，预占与消费分开记录。" }].map(item => <div key={item.title}><span className="mt-1 grid size-9 shrink-0 place-items-center rounded-xl bg-brand-soft text-brand-emphasis"><item.icon className="size-4" /></span><div><h2 className="text-sm font-semibold">{t(item.title)}</h2><p className="mt-2 text-xs leading-6 text-ink-secondary">{t(item.copy)}</p></div></div>)}</section>
    {context && <section className="mt-6 text-xs leading-6 text-ink-mute"><p>{t("品牌")} <strong className="font-medium text-ink-secondary">{context.brand.name}</strong> {t("· 当前由")} <strong className="font-medium text-ink-secondary">{context.operator.name}</strong>{context.operator.legal_name ? `（${context.operator.legal_name}）` : ""} {t("经营、收款并履约。注册时确认的经营方决定账号、套餐、余额及推广关系的归属。")}</p>{context.availability.some(item => !item.enabled) && <div className="mt-4"><Notice>{t("当前尚未开放：")}{context.availability.filter(item => !item.enabled).map(item => ({ registration: t("客户注册"), new_purchase: t("套餐新购"), renewal: t("自动续费"), model_usage: t("模型调用"), activity: t("营销活动"), settlement: t("结算") }[item.capability] || item.capability)).join(t("、"))}{t("。")} {Array.from(new Set(context.availability.filter(item => !item.enabled).map(item => item.reason ? t(item.reason) : "").filter(Boolean))).join(t("；"))}{t("。")} {Array.from(new Set(context.availability.filter(item => !item.enabled).map(item => item.next_action ? t(item.next_action) : "").filter(Boolean))).join(t("；"))}</Notice></div>}</section>}
    <section className="th-public-section"><div className="mb-6 flex flex-wrap items-end justify-between gap-3"><div><p className="th-eyebrow mb-2 text-brand-emphasis">{t("MODELS")}</p><h2 className="text-2xl font-semibold tracking-tight">{t("选择适合应用的模型")}</h2></div><Link href="/models" className="inline-flex items-center gap-2 text-sm font-medium text-brand-emphasis">{t("全部模型与价格")}<ArrowRight className="size-4" /></Link></div>{models.isPending ? <Loading /> : models.isError ? <ErrorPanel error={models.error} retry={() => void models.refetch()} /> : !models.data.items.length ? <Empty title={t("模型目录准备中")}>{t("经营方完成授权、价格和路由验证后将开放调用。")}</Empty> : <div className="grid gap-4 md:grid-cols-3">{models.data.items.slice(0, 3).map(model => <Card key={model.id} className="flex flex-col"><span className="mb-5 grid size-10 place-items-center rounded-xl bg-brand-soft text-brand-emphasis"><Layers3 className="size-5" /></span><h3 className="break-all font-semibold">{model.id}</h3><p className="text-sm text-ink-secondary">{model.capabilities.map(capability => t(capabilityLabels[capability] || capability)).join(" · ")}</p><Link href={`/workspace/models?model=${encodeURIComponent(model.id)}`} className="mt-auto inline-flex items-center gap-2 pt-5 text-sm font-medium text-brand-emphasis">{t("查看价格与能力")}<ArrowUpRight className="size-4" /></Link></Card>)}</div>}</section>
    <section className="th-public-section"><div className="mb-6 flex flex-wrap items-end justify-between gap-3"><div><p className="th-eyebrow mb-2 text-brand-emphasis">{t("PLANS")}</p><h2 className="text-2xl font-semibold tracking-tight">{t("通用额度，按需购买")}</h2></div><Link href="/plans" className="inline-flex items-center gap-2 text-sm font-medium text-brand-emphasis">{t("全部套餐与条款")}<ArrowRight className="size-4" /></Link></div>{plans.isPending ? <Loading /> : plans.isError ? <ErrorPanel error={plans.error} retry={() => void plans.refetch()} /> : !plans.data.items.length ? <Empty title={t("当前没有可购买套餐")}>{t("商品与收款方式就绪后在此展示。")}</Empty> : <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">{plans.data.items.slice(0, 3).map(plan => <PublicPlanCard key={plan.id} plan={plan} />)}</div>}</section>
  </main>;
}
export function ModelsPage() {
  const { t } = useI18n();
  const { context } = useProduct();
  const data = useQuery({ queryKey: ["public-models"], queryFn: () => request("/api/v1/public/models", collection(ModelSchema)) });
  return <main className="mx-auto max-w-[1400px] px-5 py-12 lg:px-8"><Heading eyebrow={context?.operator.name} title={t("模型与价格")} description={t("查看当前经营方开放的模型、能力与计量价格，选择后即可前往接入。")} actions={<Button asChild><Link href="/workspace/models">{t("接入指南")}<ArrowRight /></Link></Button>} />{data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <ModelCatalog models={data.data.items} />}<div className="mt-7"><ContextHelp title={t("调用价格与服务范围")}><p>{t("调用按受理时的价格快照结算，始终使用你指定的公开模型。可调用范围同时受经营方上架状态、有效权益与 Key 范围约束。")}</p><Link href="/plans" className="text-brand-emphasis underline">{t("选择通用 ATG 套餐")}</Link></ContextHelp></div></main>;
}
export function PlansPage() {
  const { t } = useI18n();
  const { context, session } = useProduct();
  const [campaign, setCampaign] = useState<CampaignLink>();
  useEffect(() => { const query = new URLSearchParams(window.location.search); setCampaign({ plan: query.get("plan") || undefined, activity: query.get("kol_activity") || undefined, invite: query.get("invite_code") || query.get("invite") || undefined }); }, []);
  const invitationQuery = campaign?.invite ? `?invite_code=${encodeURIComponent(campaign.invite)}` : "";
  const invitation = useQuery({ queryKey: ["public-plan-context", campaign?.invite], queryFn: () => request(`/api/v1/public/context${invitationQuery}`, PublicContextSchema), enabled: !!campaign?.invite && session?.kind !== "customer" });
  const data = useQuery({ queryKey: ["public-plans", campaign?.invite || ""], queryFn: () => request(`/api/v1/public/plans${invitationQuery}`, collection(PlanSchema)), enabled: campaign !== undefined });
  const operator = session?.kind === "customer" ? session.operator : invitation.data?.operator || context?.operator;
  return <main className="mx-auto max-w-[1300px] px-5 py-12 lg:px-8"><Heading eyebrow={operator?.name} title={t("通用 ATG 套餐")} description={t("为当前经营方的开放模型购买通用服务额度。可直接购包，无须先充值。")} actions={<Button asChild variant="outline"><Link href="/models">{t("先查看模型与价格")}</Link></Button>} />{campaign?.activity && <div className="mb-5"><Notice><p>{t("你正在查看分享的套餐活动。登录后会核实直属邀请关系、期限与剩余名额。")}</p><p className="mt-2">{t("已有账号的经营方和邀请人不会因打开链接而改变。")}</p></Notice></div>}{operator && <div className="mb-6 rounded-control border border-hairline p-4 text-sm"><p>{t("实际经营方：")} <strong>{operator.name}</strong>{operator.legal_name ? ` · ${operator.legal_name}` : ""}<FieldHelp title={t("调用价格与服务范围")}><p>{t("收款并负责交付，价格包含相关税费。最终活动权益、有效期和支付方式在结算前统一确认。")}</p></FieldHelp></p></div>}{data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : !data.data.items.length ? <Empty title={t("当前没有可购买的套餐")}>{t("经营方完成商品与收款配置后会在这里展示。")}</Empty> : <><div className="grid gap-5 md:grid-cols-2 lg:grid-cols-3">{data.data.items.map(plan => <PublicPlanCard key={plan.id} plan={plan} campaign={campaign} />)}</div>{campaign?.plan && !data.data.items.some(item => item.id === campaign.plan) && <div className="mt-5"><Notice>{t("链接中的套餐当前不可购买，请选择目录中的可用套餐。")}</Notice></div>}</>}{invitation.isError && <div className="mt-5"><ErrorPanel error={invitation.error} retry={() => void invitation.refetch()} /></div>}<div className="mt-8 flex flex-wrap gap-6 text-sm text-ink-secondary"><div>{t("套餐怎样生效与到期")}<FieldHelp title={t("套餐怎样生效与到期")}><p>{t("足额付款并成功交付后生效。每份套餐独立计算有效期，月、季、年均从交付日起按日历期间计算。多份可并存，正常到期余量不结转。")}</p><p>{t("自动续费需要单独授权；提前用完不会提前自动续费，续费条件变差须重新确认。")}</p></FieldHelp></div><div>{t("付款、退款与使用记录")}<FieldHelp title={t("付款、退款与使用记录")}><p>{t("套餐不提供一般性退款；重复、错误交易、未交付或无法履约的问题通过异常流程办理。未使用且未占用的 结算币种 支付余额可申请退回。")}</p><p>{t("购买后在工作台创建 Key、获取接入代码，并在用量账单查看价格与费用依据。")}</p></FieldHelp></div></div></main>;
}
