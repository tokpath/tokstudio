"use client";
import { useI18n } from "@/components/i18n";
import { useId, useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { TakeoversSchema, TakeoverSchema, type Takeover, Decimal, SignedDecimal } from "@/lib/contracts";
import { useProduct } from "./context";
import { useMonetary } from "./monetary";
import { canReadResource } from "@/lib/navigation";
import { compareDecimal } from "@/lib/decimal";
import { useCommand, Heading, Loading, ErrorPanel, Empty, Money, Notice, Status, Table, When, Stat } from "./shared";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { FieldHelp } from "./field-help";
import { businessFieldHelp } from "./business-field-help";
import { OperatorPanel, ReadFacts, resourceValueText } from "./operator-resources";

export function TakeoversScreen({operatorID}:{operatorID:string}){
  const { t } = useI18n();
 const {session}=useProduct();const scope=`/api/v1/operators/${encodeURIComponent(operatorID)}`;
 const workspace=session?.kind==="staff"?session.workspaces.find(w=>w.operator.id===operatorID):undefined;
 const canRead=canReadResource(workspace,"takeovers");
 const has=(permission:string)=>!!workspace&&(workspace.permissions.includes("*")||workspace.permissions.includes(permission));
 const canPrepare=has("cooperation.manage"),canExecute=canPrepare&&has("refunds.manage");
 const data=useQuery({queryKey:["takeovers",operatorID],queryFn:()=>request(`${scope}/takeovers`,TakeoversSchema),enabled:canRead});
 const [creating,setCreating]=useState(false);
 if(!canRead)return <Empty title={t("没有查看权限")}>{t("需要品牌经营方的合作与退款权限。")}</Empty>;
 if(data.isPending)return <Loading/>;if(data.isError)return <ErrorPanel error={data.error} retry={()=>void data.refetch()}/>;
 const items=data.data.items;
 return <><Heading title={t("渠道接管")} description={t("从接管准备、客户权益承接到双方资金结清，在同一事项中连续办理。")} actions={canExecute&&<Button onClick={()=>setCreating(true)}>{t("发起接管准备")}</Button>}/>
 {creating&&<BeginTakeover scope={scope} channels={data.data.channels} close={()=>setCreating(false)}/>}
 {!items.length&&<Empty>{t("尚未建立接管方案。")}</Empty>}
 <div className="space-y-7">{items.map(item=><TakeoverCard key={item.id} item={item} scope={scope} channelName={data.data.channels.find(c=>c.id===item.channel_id)?.name||item.channel_id} canPrepare={canPrepare} canExecute={canExecute}/>)}</div>
 </>;
}
const takeoverHelp: Record<string, string> = {
 service_money_per_atg: "按双方接管合同填写服务责任每 ATG 的 结算币种 作价。例如 100 ATG、每 ATG 作价 0.02 结算币种，对应服务作价 2 结算币种；最终金额依完整责任快照核实，填写不会发起实际付款。",
 procurement_money_per_atg: "这是采购库存抵扣时每 ATG 的合同 结算币种 作价，与承接服务作价分别约定。系统按真实库存和合同规则核算抵扣，不会因填写此价格增加库存或直接退款。",
 extra_money: "正数增加渠道向品牌的合同结清额，负数减少该金额，并可能改变付款方向。没有额外约定填写 0；请在完整快照核对最终结清方向。",
 contract_reference: "填写双方已确认的接管合同编号或文件引用，用于核对本次承接范围、作价、库存退出和结清责任。创建方案不会代替双方签署合同。",
 actual_money: "填写本次实际清偿的 结算币种 金额，手续费单独记录。例如到账结清 9.8 结算币种、付款方另付手续费 0.2 结算币种，此处填 9.8，手续费填 0.2；原币执行事实与已核实的结算金额分别记录，不填写汇率。",
 reference: "有转账流水或现金收据号时填写；没有可留空，系统自动生成登记编号。请勿重复登记同一笔款项。",
};
function Field({label,name,required=true,initial="",type="text",onChange}:{label:string;name:string;required?:boolean;initial?:string;type?:string;onChange?:(value:string)=>void}){
  const { t } = useI18n(); const id = useId(); const help = takeoverHelp[name] || businessFieldHelp(name); return <div className="text-sm"><label htmlFor={id}>{t(label)}</label>{help && <FieldHelp title={t(label)}>{t(help)}</FieldHelp>}<Input id={id} name={name} type={type} required={required} defaultValue={initial} onChange={event=>onChange?.(event.target.value)} className="mt-2"/></div>}
function BeginTakeover({scope,channels,close}:{scope:string;channels:{id:string;name:string;legal_name:string}[];close:()=>void}){
  const { t } = useI18n();
 const command=useCommand(`${scope}/takeovers`,TakeoverSchema);const [validation,setValidation]=useState("");
 async function submit(event:FormEvent<HTMLFormElement>){event.preventDefault();if(command.unresolved){if(await command.run({}))close();return}const f=new FormData(event.currentTarget);const input=Object.fromEntries(f.entries());if(!Decimal.safeParse(input.service_money_per_atg).success||!Decimal.safeParse(input.procurement_money_per_atg).success||!SignedDecimal.safeParse(input.extra_money).success){setValidation("金额须为精确十进制字符串。");return}setValidation("");if(await command.run(input))close()}
 return <OperatorPanel open title={t("准备接管方案")} description={t("选择直属渠道并保存双方合同条款，先暂停新增业务再复核存量责任。")} blocked={command.busy||command.unresolved} close={close}><form onSubmit={submit} className="mt-5 space-y-5"><fieldset disabled={command.busy||command.unresolved} className="grid gap-5 md:grid-cols-2">
 <label className="text-sm">{t("直属渠道")}<select required name="channel_id" className="mt-2 w-full rounded-control border border-hairline bg-canvas-raised p-3"><option value="">{t("请选择")}</option>{channels.map(c=><option key={c.id} value={c.id}>{c.name} · {c.legal_name}</option>)}</select></label>
 <label className="text-sm">{t("合同作价方式")}<select required name="method" className="mt-2 w-full rounded-control border border-hairline bg-canvas-raised p-3"><option value="supply_coverage">{t("按供货覆盖计算")}</option><option value="agreed_atg_value">{t("按约定ATG价格")}</option></select></label>
 <Field name="service_money_per_atg" label={t("承接服务每ATG合同作价 结算币种（大于0）")}/><Field name="procurement_money_per_atg" label={t("采购库存每ATG合同抵扣 结算币种（大于0）")}/><Field name="extra_money" label={t("合同额外结清 结算币种（可负数）")} initial="0"/><Field name="contract_reference" label={t("双方接管合同编号")}/><Field name="evidence" label={t("发起依据与证据引用")}/>
 <label className="md:col-span-2 text-sm leading-7"><input required type="checkbox" className="mr-3"/>{t("确认暂停此渠道新增注册、购买、调用和自动补货。取消方案后也需要另行检查再恢复营业。")}</label></fieldset>
 {validation&&<p role="alert" className="text-danger">{t(validation)}</p>}{command.unresolved&&<Notice>{t("方案办理结果待核实，请保持此面板。重试沿用原始填写内容和同一操作编号。")}</Notice>}{!!command.error&&<ErrorPanel error={command.error}/>}<Button disabled={command.busy}>{command.unresolved?t("重试原方案"):t("建立准备事项")}</Button></form></OperatorPanel>
}
function TakeoverCard({item,scope,channelName,canPrepare,canExecute}:{item:Takeover;scope:string;channelName:string;canPrepare:boolean;canExecute:boolean}){
  const { t } = useI18n();
 const [details,setDetails]=useState(false),[action,setAction]=useState<"execute"|"cancel"|"settle">();
 const refresh=useCommand(`${scope}/takeovers/${item.id}/refresh`,TakeoverSchema);
 const snapshot=item.snapshot;const payable=item.settlement_due_money.replace(/^-/g,"");const fullySettled=compareDecimal(item.settled_money,payable)>=0;const stages=[{label:t("暂停与排空"),done:["awaiting_approval","completed"].includes(item.state)},{label:t("完整责任复核"),done:item.state==="completed"},{label:t("权益承接完成"),done:item.state==="completed"},{label:t("实际资金结清"),done:item.state==="completed"&&fullySettled}];const direction=item.settlement_due_money.startsWith("-")?t("品牌向渠道付款"):t("渠道向品牌付款");
 return <Card><div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="text-xl font-semibold">{channelName}</h2><p className="mt-2 text-sm">{t("合同")} {item.contract_reference} · <When value={item.created_at}/></p></div><Status value={item.state}/></div>
 <ol aria-label={t("接管办理进度")} className="mt-6 grid gap-2 sm:grid-cols-4">{stages.map((stage,index)=><li key={stage.label} className={`rounded-control border p-3 text-xs ${stage.done?"border-success/30 text-success":"border-hairline text-ink-mute"}`}><span className="mr-2 font-mono">{index+1}</span>{stage.label}{stage.done&&<span className="ml-2">{t("已完成")}</span>}</li>)}</ol><div className="my-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><Stat label={t("承接服务作价")} value={item.service_money} unit="MONEY"/><Stat label={t("承接现金与佣金责任")} value={item.cash_money} unit="MONEY"/><Stat label={t("采购库存抵扣")} value={item.procurement_offset_money} unit="MONEY" detail={`${item.procurement_offset_atg} ATG`}/><Stat label={t("合同可退采购余额")} value={item.remaining_procurement_refund_money} unit="MONEY"/></div>
 <Notice><p>{t("结清方向：{direction}，合同结清额", { direction })} <Money value={payable} unit="MONEY"/>{t("；已登记结清")} <Money value={item.settled_money} unit="MONEY"/>{t("。")}</p><p>{t("客户权益转移与双方现金支付分别记账。收款方资产、原付款来源和历史凭证保留；不因点击办理推定已收付款。")}</p></Notice>
 <div className="mt-5 flex flex-wrap gap-3">{snapshot&&<Button variant="outline" onClick={()=>setDetails(v=>!v)}>{details?t("收起完整快照"):t("审阅完整快照")}</Button>}{canPrepare&&["preparing","awaiting_approval"].includes(item.state)&&<><Button disabled={refresh.busy} variant="outline" onClick={()=>void refresh.run({})}>{refresh.unresolved?t("重试原快照检查"):t("排空检查并刷新快照")}</Button><Button variant="outline" disabled={refresh.busy||refresh.unresolved} onClick={()=>setAction("cancel")}>{t("取消方案")}</Button></>}{canExecute&&item.state==="awaiting_approval"&&snapshot&&<Button disabled={refresh.busy||refresh.unresolved} onClick={()=>{setDetails(true);setAction("execute")}}>{t("复核并执行接管")}</Button>}{canExecute&&item.state==="completed"&&!fullySettled&&<Button onClick={()=>setAction("settle")}>{t("登记实际收付款")}</Button>}</div>
 {!!refresh.error&&<div className="mt-4"><ErrorPanel error={refresh.error}/></div>}
 {details&&snapshot&&<TakeoverSnapshot item={item}/>}
 {action&&<TakeoverAction key={action} action={action} scope={scope} item={item} direction={direction} close={()=>setAction(undefined)}/>}
 </Card>
}
function TakeoverSnapshot({item}:{item:Takeover}){
  const { t, locale } = useI18n();
 const snapshot=item.snapshot;if(!snapshot)return null;
 return <div className="my-6 space-y-6"><h3 className="font-semibold">{t("完整责任快照 ·")} <span className="break-all font-mono text-xs">{item.snapshot_fingerprint}</span></h3><Table rows={Object.entries(snapshot.customers).map(([id,version])=>({id,version}))} getID={r=>r.id} columns={[{label:t("完整客户范围"),cell:r=><span className="break-all">{r.id}</span>},{label:t("归属版本"),cell:r=>r.version}]}/><Table rows={snapshot.assets} getID={r=>r.account_id} columns={[{label:t("客户 / 账户"),cell:r=><span className="break-all">{r.customer_id}<br/>{r.account_id}</span>},{label:t("资产目的"),cell:r=>resourceValueText(r.purpose,"purpose",locale)},{label:t("总余额"),cell:r=><Money value={r.balance} unit={r.unit}/>},{label:t("占用"),cell:r=><Money value={r.held} unit={r.unit}/>},{label:t("版本"),cell:r=>r.version}]}/><Facts title={t("覆盖与责任计算")} value={snapshot.coverage}/><Facts title={t("采购库存原始头寸")} value={snapshot.procurement}/><Facts title={t("每笔采购批次及合同退出条款")} value={snapshot.lots}/><Facts title={t("计价与舍入依据")} value={{service_atg:snapshot.service_atg,service_money:snapshot.service_money,cash_money:snapshot.cash_money,offset_atg:snapshot.offset_atg,offset_money:snapshot.offset_money,residual_refund_money:snapshot.residual_refund_money,settlement_due_money:snapshot.settlement_due_money,...snapshot.rounding_evidence}}/></div>;
}
function Facts({title,value}:{title:string;value:unknown}) {
 const { t } = useI18n();
 return <div className="overflow-hidden rounded-control border border-hairline p-4"><h4 className="mb-3 text-sm font-semibold">{t(title)}</h4><ReadFacts value={value as import("@/lib/contracts").JSONValue}/></div>;
}
function TakeoverAction({action,scope,item,direction,close}:{action:"execute"|"cancel"|"settle";scope:string;item:Takeover;direction:string;close:()=>void}){
  const { t } = useI18n();
 const command=useCommand(`${scope}/takeovers/${item.id}/${action}`,TakeoverSchema);
 const monetary = useMonetary();
 const [validation,setValidation]=useState("");const [currency,setCurrency]=useState(monetary.currency);
 async function submit(event:FormEvent<HTMLFormElement>){event.preventDefault();if(command.unresolved){if(await command.run({}))close();return}const f=new FormData(event.currentTarget);const input:Record<string,unknown>=Object.fromEntries(f.entries());if(action==="execute")input.expected_snapshot=item.snapshot_fingerprint;if(action==="settle"){input.currency=String(input.currency).trim().toUpperCase();if(input.currency===monetary.currency)input.actual_money=input.amount;const at=new Date(String(input.occurred_at));if(Number.isNaN(at.getTime())||!Decimal.safeParse(input.amount).success||!Decimal.safeParse(input.actual_money).success||!Decimal.safeParse(input.fee_money).success){setValidation("请填写有效收付款时间及精确十进制金额。");return}input.occurred_at=at.toISOString()}setValidation("");if(await command.run(input))close()}
 return <OperatorPanel open title={action==="execute"?t("复核完整责任并执行"):action==="cancel"?t("取消准备方案"):t("登记本次实际结清")} description={t("渠道接管 {id} · 合同 {contract}", { id: item.id, contract: item.contract_reference })} blocked={command.busy||command.unresolved} close={close}>{action==="execute"&&<TakeoverSnapshot item={item}/>}<form onSubmit={submit} className="space-y-5"><fieldset disabled={command.busy||command.unresolved} className="grid gap-5 md:grid-cols-2">
 {action==="cancel"&&<Field name="evidence" label={t("取消依据")}/>}
 {action==="settle"&&<><Notice>{t("{direction}。分别核实原币收付款事实、结算金额和付款方手续费，不填写汇率。", { direction })}</Notice><Field name="currency" label={t("实际币种")} initial={monetary.currency} onChange={setCurrency}/><Field name="amount" label={t("实际币种金额")}/>{currency.trim().toUpperCase()!==monetary.currency&&<Field name="actual_money" label={t("本次实际结清 结算币种")}/>}<Field name="fee_money" label={t("付款方另付手续费 结算币种")} initial="0"/><Field name="reference" label={t("收付款流水或收据号（可选）")} required={false}/><Field name="evidence" label={t("收付款核实依据")}/><Field name="occurred_at" label={t("实际收付款时间")} type="datetime-local"/></>}
 <label className="md:col-span-2 text-sm leading-7"><input required type="checkbox" className="mr-3"/>{action==="execute"?t("我已审阅上述完整客户、四类资产、采购批次、覆盖计算、合同及正负结清额。服务器会重新核对快照并执行独立复核策略。"):action==="cancel"?t("确认取消方案；不会自动恢复渠道营业。"):t("已核实本次实际收付款，登记内容真实且未重复记录。")}</label></fieldset>
 {validation&&<p role="alert" className="text-danger">{t(validation)}</p>}{command.unresolved&&<Notice>{t("本次办理结果待核实，请保持此面板。重试沿用原始凭证和同一操作编号，不重复办理。")}</Notice>}{!!command.error&&<ErrorPanel error={command.error}/>}<div className="flex gap-3"><Button disabled={command.busy}>{command.unresolved?t("重试原操作"):t("确认提交")}</Button><Button type="button" variant="outline" disabled={command.unresolved||command.busy} onClick={close}>{t("返回")}</Button></div></form></OperatorPanel>
}
