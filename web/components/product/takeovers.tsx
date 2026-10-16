"use client";
import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { TakeoversSchema, TakeoverSchema, type Takeover, Decimal, SignedDecimal } from "@/lib/contracts";
import { useProduct } from "./context";
import { useCommand, Heading, Loading, ErrorPanel, Empty, Money, Notice, Status, Table, When, Stat } from "./shared";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";

export function TakeoversScreen({operatorID,settlements=false}:{operatorID:string;settlements?:boolean}){
 const {session}=useProduct();const scope=`/api/v1/operators/${encodeURIComponent(operatorID)}`;
 const workspace=session?.kind==="staff"?session.workspaces.find(w=>w.operator.id===operatorID):undefined;
 const canRead=!!workspace&&(workspace.permissions.includes("*")||workspace.permissions.includes("takeovers.read"));
 const data=useQuery({queryKey:["takeovers",operatorID],queryFn:()=>request(`${scope}/takeovers`,TakeoversSchema),enabled:canRead});
 const [creating,setCreating]=useState(false);
 if(!canRead)return <Empty title="没有查看权限">需要品牌经营方的合作与退款权限。</Empty>;
 if(data.isPending)return <Loading/>;if(data.isError)return <ErrorPanel error={data.error} retry={()=>void data.refetch()}/>;
 const items=settlements?data.data.items.filter(item=>item.state==="completed"):data.data.items;
 return <><Heading title={settlements?"接管资金结清":"渠道接管"} description={settlements?"客户权益接管完成后，双方实际收付款仍独立办理。登记真实银行凭证与当次USD估值，支持分批结清。":"先暂停新增业务，等待在途事项结清，再审阅完整资产与责任快照。执行接管不会继承原渠道商户密钥。"} actions={!settlements&&<Button onClick={()=>setCreating(v=>!v)}>发起接管准备</Button>}/>
 {creating&&!settlements&&<BeginTakeover scope={scope} channels={data.data.channels} close={()=>setCreating(false)}/>}
 {!items.length&&<Empty>{settlements?"暂无已完成接管的结清事项。":"尚未建立接管方案。"}</Empty>}
 <div className="space-y-7">{items.map(item=><TakeoverCard key={item.id} item={item} scope={scope} channelName={data.data.channels.find(c=>c.id===item.channel_id)?.name||item.channel_id} settlements={settlements}/>)}</div>
 </>;
}
function Field({label,name,required=true,initial="",type="text"}:{label:string;name:string;required?:boolean;initial?:string;type?:string}){return <label className="block text-sm">{label}<Input name={name} type={type} required={required} defaultValue={initial} className="mt-2"/></label>}
function BeginTakeover({scope,channels,close}:{scope:string;channels:{id:string;name:string;legal_name:string}[];close:()=>void}){
 const command=useCommand(`${scope}/takeovers`,TakeoverSchema);const [validation,setValidation]=useState("");
 async function submit(event:FormEvent<HTMLFormElement>){event.preventDefault();const f=new FormData(event.currentTarget);const input=Object.fromEntries(f.entries());if(!Decimal.safeParse(input.service_usd_per_atg).success||!Decimal.safeParse(input.procurement_usd_per_atg).success||!SignedDecimal.safeParse(input.extra_usd).success){setValidation("金额须为精确十进制字符串。");return}setValidation("");if(await command.run(input))close()}
 return <Card className="mb-7"><h2 className="text-xl font-semibold">准备接管方案</h2><form onSubmit={submit} className="mt-5 space-y-5"><fieldset disabled={command.busy||command.unresolved} className="grid gap-5 md:grid-cols-2">
 <label className="text-sm">直属渠道<select required name="channel_id" className="mt-2 w-full rounded-control border border-hairline bg-canvas-raised p-3"><option value="">请选择</option>{channels.map(c=><option key={c.id} value={c.id}>{c.name} · {c.legal_name}</option>)}</select></label>
 <label className="text-sm">合同作价方式<select required name="method" className="mt-2 w-full rounded-control border border-hairline bg-canvas-raised p-3"><option value="supply_coverage">按供货覆盖计算</option><option value="agreed_atg_value">按约定ATG价格</option></select></label>
 <Field name="service_usd_per_atg" label="承接服务每ATG合同作价 USD（大于0）"/><Field name="procurement_usd_per_atg" label="采购库存每ATG合同抵扣 USD（大于0）"/><Field name="extra_usd" label="合同额外结清 USD（可负数）" initial="0"/><Field name="contract_reference" label="双方接管合同编号"/><Field name="evidence" label="发起依据与证据引用"/>
 <label className="md:col-span-2 text-sm leading-7"><input required type="checkbox" className="mr-3"/>确认暂停此渠道新增注册、购买、调用和自动补货。取消方案后也需要另行检查再恢复营业。</label></fieldset>
 {validation&&<p role="alert" className="text-danger">{validation}</p>}{!!command.error&&<ErrorPanel error={command.error}/>}<Button disabled={command.busy}>{command.unresolved?"重试原方案":"建立准备事项"}</Button></form></Card>
}
function TakeoverCard({item,scope,channelName,settlements}:{item:Takeover;scope:string;channelName:string;settlements:boolean}){
 const [details,setDetails]=useState(false),[action,setAction]=useState<"execute"|"cancel"|"settle">();
 const refresh=useCommand(`${scope}/takeovers/${item.id}/refresh`,TakeoverSchema);
 const snapshot=item.snapshot;const direction=item.settlement_due_usd.startsWith("-")?"品牌向渠道付款":"渠道向品牌付款";
 return <Card><div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="text-xl font-semibold">{channelName}</h2><p className="mt-2 break-all font-mono text-xs text-ink-mute">{item.id}</p><p className="mt-2 text-sm">合同 {item.contract_reference} · <When value={item.created_at}/></p></div><Status value={item.state}/></div>
 <div className="my-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><Stat label="承接服务作价" value={item.service_usd} unit="USD"/><Stat label="承接现金与佣金责任" value={item.cash_usd} unit="USD"/><Stat label="采购库存抵扣" value={item.procurement_offset_usd} unit="USD" detail={`${item.procurement_offset_atg} ATG`}/><Stat label="合同可退采购余额" value={item.remaining_procurement_refund_usd} unit="USD"/></div>
 <Notice><p>结清方向：{direction}，合同结清额 <Money value={item.settlement_due_usd} unit="USD"/>；已登记结清 <Money value={item.settled_usd} unit="USD"/>。</p><p>客户权益转移与双方现金支付分别记账。收款方资产、原付款来源和历史凭证保留；不因点击办理推定已收付款。</p></Notice>
 <div className="mt-5 flex flex-wrap gap-3">{snapshot&&<Button variant="outline" onClick={()=>setDetails(v=>!v)}>{details?"收起完整快照":"审阅完整快照"}</Button>}{!settlements&&["preparing","awaiting_approval"].includes(item.state)&&<><Button disabled={refresh.busy} variant="outline" onClick={()=>void refresh.run({})}>排空检查并刷新快照</Button><Button variant="outline" onClick={()=>setAction("cancel")}>取消方案</Button></>}{!settlements&&item.state==="awaiting_approval"&&snapshot&&<Button onClick={()=>{setDetails(true);setAction("execute")}}>复核并执行接管</Button>}{settlements&&item.state==="completed"&&<Button onClick={()=>setAction("settle")}>登记实际收付款</Button>}</div>
 {!!refresh.error&&<div className="mt-4"><ErrorPanel error={refresh.error}/></div>}
 {details&&snapshot&&<div className="mt-6 space-y-6"><h3 className="font-semibold">待执行快照 · <span className="break-all font-mono text-xs">{item.snapshot_fingerprint}</span></h3><Table rows={Object.entries(snapshot.customers).map(([id,version])=>({id,version}))} getID={r=>r.id} columns={[{label:"完整客户范围",cell:r=><span className="break-all">{r.id}</span>},{label:"归属版本",cell:r=>r.version}]}/><Table rows={snapshot.assets} getID={r=>r.account_id} columns={[{label:"客户 / 账户",cell:r=><span className="break-all">{r.customer_id}<br/>{r.account_id}</span>},{label:"资产目的",cell:r=>r.purpose},{label:"总余额",cell:r=><Money value={r.balance} unit={r.unit}/>},{label:"占用",cell:r=><Money value={r.held} unit={r.unit}/>},{label:"版本",cell:r=>r.version}]}/><Facts title="覆盖与责任计算" value={snapshot.coverage}/><Facts title="采购库存原始头寸" value={snapshot.procurement}/><Facts title="每笔采购批次及合同退出条款" value={snapshot.lots}/><Facts title="计价与舍入依据" value={{service_atg:snapshot.service_atg,service_usd:snapshot.service_usd,cash_usd:snapshot.cash_usd,offset_atg:snapshot.offset_atg,offset_usd:snapshot.offset_usd,residual_refund_usd:snapshot.residual_refund_usd,settlement_due_usd:snapshot.settlement_due_usd,...snapshot.rounding_evidence}}/></div>}
 {action&&<TakeoverAction key={action} action={action} scope={scope} item={item} direction={direction} close={()=>setAction(undefined)}/>}
 </Card>
}
function Facts({title,value}:{title:string;value:unknown}){return <div className="overflow-hidden rounded-control border border-hairline p-4"><h4 className="mb-3 text-sm font-semibold">{title}</h4>{Array.isArray(value)?value.length?value.map((item,i)=><Facts key={i} title={`批次 ${i+1}`} value={item}/>):<p className="text-sm text-ink-mute">无</p>:value!==null&&typeof value==="object"?<dl className="grid gap-x-6 gap-y-3 md:grid-cols-2">{Object.entries(value).map(([key,v])=><div key={key} className="min-w-0 text-xs"><dt className="text-ink-mute">{key}</dt><dd className="mt-1 break-words font-mono">{v!==null&&typeof v==="object"?<Facts title={key} value={v}/>:String(v??"—")}</dd></div>)}</dl>:<span>{String(value??"—")}</span>}</div>}
function TakeoverAction({action,scope,item,direction,close}:{action:"execute"|"cancel"|"settle";scope:string;item:Takeover;direction:string;close:()=>void}){
 const command=useCommand(`${scope}/takeovers/${item.id}/${action}`,TakeoverSchema);
 async function submit(event:FormEvent<HTMLFormElement>){event.preventDefault();const f=new FormData(event.currentTarget);const input:Record<string,unknown>=Object.fromEntries(f.entries());if(action==="execute")input.expected_snapshot=item.snapshot_fingerprint;if(action==="settle"){input.occurred_at=new Date(String(input.occurred_at)).toISOString();if(!Decimal.safeParse(input.actual_usd).success||!Decimal.safeParse(input.fee_usd).success)return}if(await command.run(input))close()}
 return <form onSubmit={submit} className="mt-6 space-y-5 border-t border-hairline pt-6"><h3 className="font-semibold">{action==="execute"?"复核完整责任并执行":action==="cancel"?"取消准备方案":"登记本次实际结清"}</h3><fieldset disabled={command.busy||command.unresolved} className="grid gap-5 md:grid-cols-2">
 {action==="cancel"&&<Field name="evidence" label="取消依据"/>}
 {action==="settle"&&<><Notice>{direction}。填写实际执行币种和金额、本次折合USD以及付款方手续费；不得使用原订单汇率代替本次真实执行事实。</Notice><Field name="currency" label="实际币种" initial="USD"/><Field name="amount" label="实际币种金额"/><Field name="actual_usd" label="本次实际结清 USD"/><Field name="fee_usd" label="付款方另付手续费 USD" initial="0"/><Field name="reference" label="唯一银行流水 / 支付凭证"/><Field name="evidence" label="核实依据与当次估值证据"/><Field name="occurred_at" label="实际收付款时间" type="datetime-local"/></>}
 <label className="md:col-span-2 text-sm leading-7"><input required type="checkbox" className="mr-3"/>{action==="execute"?"我已审阅上述完整客户、四类资产、采购批次、覆盖计算、合同及正负结清额。服务器会重新核对快照并执行独立复核策略。":action==="cancel"?"确认取消方案；不会自动恢复渠道营业。":"已向银行或支付通道核实本次实际收付款，凭证真实且未重复登记。"}</label></fieldset>
 {!!command.error&&<ErrorPanel error={command.error}/>}<div className="flex gap-3"><Button disabled={command.busy}>{command.unresolved?"重试原操作":"确认提交"}</Button><Button type="button" variant="outline" disabled={command.unresolved||command.busy} onClick={close}>返回</Button></div></form>
}
