"use client";
import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { request, newActionKey } from "@/lib/api-client";
import { TakeoversSchema, TakeoverSchema, CommandResultSchema, type ExitNotice, type Takeover } from "@/lib/contracts";
import { canReadResource } from "@/lib/navigation";
import { useProduct } from "./context";
import { useCommand, Heading, Loading, ErrorPanel, Empty, Money, Notice, Status, Table, When, Stat } from "./shared";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { OperatorPanel } from "./operator-resources";

export function TakeoversScreen({operatorID}:{operatorID:string}) {
 const {t}=useI18n(); const {session}=useProduct();
 const scope=`/api/v1/operators/${encodeURIComponent(operatorID)}`;
 const workspace=session?.kind==="staff"?session.workspaces.find(w=>w.operator.id===operatorID):undefined;
 const canRead=canReadResource(workspace,"takeovers");
 const canManage=workspace?.operator.kind!=="channel" && !!workspace?.permissions.some(p=>p==="*"||p==="cooperation.manage");
 const query=useQuery({queryKey:["takeovers",operatorID],queryFn:()=>request(`${scope}/takeovers`,TakeoversSchema),enabled:canRead});
 const [creating,setCreating]=useState<string>(),[notifying,setNotifying]=useState(false);
 if(!canRead)return <Empty title={t("没有查看权限")}/>;
 if(query.isPending)return <Loading/>;
 if(query.isError)return <ErrorPanel error={query.error} retry={()=>void query.refetch()}/>;
 const data=query.data;
 const notifyLabel=workspace?.operator.kind==="oem"?"告知平台停止经营":"通知品牌办理退出";
 const standalone=data.notices.filter(n=>!data.items.some(item=>item.channel_id===n.operator_id));
 return <><Heading title={t("停业与接管")} description={t("查看停业告知、渠道接管进度和结果。采购余额不退，双方资金交接在线下处理。")} actions={<div className="flex flex-wrap gap-2"><Button variant="outline" onClick={()=>void query.refetch()}>{t("刷新")}</Button>{data.can_notify&&<Button onClick={()=>setNotifying(true)}>{t(notifyLabel)}</Button>}{canManage&&data.channels.length>0&&<Button onClick={()=>setCreating("")}>{t("接管渠道")}</Button>}</div>}/>
 {notifying&&<ExitNoticeForm scope={scope} oem={workspace?.operator.kind==="oem"} close={()=>setNotifying(false)}/>}
 {creating!==undefined&&<BeginTakeover scope={scope} channels={data.channels} selected={creating} close={()=>setCreating(undefined)}/>}
 {!data.items.length&&!data.notices.length&&<Empty>{t("暂无停业或接管事项。")}</Empty>}
 <div className="space-y-6">{standalone.map(n=><Card key={n.id}><h2 className="text-xl font-semibold">{n.operator_name}</h2><NoticeDetails notice={n}/>{n.kind==="channel"&&canManage&&data.channels.some(c=>c.id===n.operator_id)&&<Button className="mt-4" onClick={()=>setCreating(n.operator_id)}>{t("接管渠道")}</Button>}</Card>)}
 {data.items.map((item,index)=><TakeoverCard key={item.id} item={item} notice={data.items.findIndex(row=>row.channel_id===item.channel_id)===index?data.notices.find(n=>n.operator_id===item.channel_id):undefined} scope={scope} canManage={canManage} channelName={item.channel_name||data.channels.find(c=>c.id===item.channel_id)?.name||t("渠道")}/>)}</div></>;
}
function NoticeDetails({notice}:{notice:ExitNotice}) {
 const {t}=useI18n();return <div className="my-3 space-y-2 text-sm"><p>{t(notice.kind==="oem"?"OEM 停业告知":"渠道退出通知")} · <When value={notice.created_at}/></p>{notice.note&&<p>{notice.note}</p>}{notice.kind==="oem"&&<p className="text-ink-mute">{t("仅作停业告知，无需平台审批。客户及下属渠道的存量责任仍由本 OEM 处理。")}</p>}</div>;
}
function ExitNoticeForm({scope,oem,close}:{scope:string;oem:boolean;close:()=>void}) {
 const {t}=useI18n();const command=useCommand(`${scope}/exits`,CommandResultSchema);
 const label=oem?"告知平台停止经营":"通知品牌办理退出";
 async function submit(event:FormEvent<HTMLFormElement>){event.preventDefault();const note=String(new FormData(event.currentTarget).get("note")||"");if(await command.run({note}))close()}
 return <OperatorPanel open title={t(label)} description={t("采购余额不退。停业告知不代替客户及存量责任的处理。")} blocked={command.busy||command.unresolved} close={close}><form onSubmit={submit} className="space-y-5"><fieldset disabled={command.busy||command.unresolved} className="space-y-5"><label className="block text-sm">{t("补充说明（可选）")}<textarea name="note" maxLength={2000} rows={3} className="mt-2 w-full rounded-control border border-hairline bg-canvas-raised p-3"/></label><label className="block text-sm leading-7"><input type="checkbox" required className="mr-2"/>{t(oem?"仅告知平台，不申请退款或接管。营业设置和客户、下属渠道的存量责任由本 OEM 自行处理。":"通知所属品牌办理客户接管。采购余额不退；接管前继续由本渠道负责客户业务。")}</label></fieldset>{!!command.error&&<ErrorPanel error={command.error}/>}<Button disabled={command.busy}>{t(command.unresolved?"重试原操作":label)}</Button></form></OperatorPanel>;
}

function BeginTakeover({scope,channels,selected,close}:{scope:string;channels:{id:string;name:string}[];selected:string;close:()=>void}){
 const {t}=useI18n();const command=useCommand(`${scope}/takeovers`,TakeoverSchema);
 async function submit(e:FormEvent<HTMLFormElement>){e.preventDefault();const fields=new FormData(e.currentTarget);if(await command.run(Object.fromEntries(fields.entries())))close()}
 return <OperatorPanel open title={t("接管渠道")} description={t("暂停新增业务，等待已受理事项完成后，核对并承接客户权益。")} blocked={command.busy||command.unresolved} close={close}>
 <form onSubmit={submit} className="space-y-5"><fieldset disabled={command.busy||command.unresolved} className="space-y-5">
 <label className="block text-sm">{t("直属渠道")}<select required name="channel_id" defaultValue={selected} className="mt-2 w-full rounded-control border border-hairline bg-canvas-raised p-3"><option value="">{t("请选择")}</option>{channels.map(c=><option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
 <label className="block text-sm">{t("补充说明（可选）")}<textarea name="evidence" maxLength={2000} rows={3} className="mt-2 w-full rounded-control border border-hairline bg-canvas-raised p-3"/></label>
 <label className="block text-sm leading-7"><input required type="checkbox" className="mr-2"/>{t("确认暂停渠道新增注册、销售、续费、调用和补货。取消接管后，由渠道负责人检查并恢复营业。")}</label>
 </fieldset>{!!command.error&&<ErrorPanel error={command.error}/>}<Button disabled={command.busy}>{command.unresolved?t("重试原操作"):t("开始准备")}</Button></form>
 </OperatorPanel>;
}
function TakeoverCard({item,notice,scope,channelName,canManage}:{item:Takeover;notice?:ExitNotice;scope:string;channelName:string;canManage:boolean}){
 const {t}=useI18n();const [action,setAction]=useState<"execute"|"cancel">();
 // Inspection is repeatable; it neither transfers assets nor confirms takeover.
 const check=useQuery({queryKey:["takeover-check",item.id,item.state],queryFn:()=>request(`${scope}/takeovers/${item.id}/refresh`,TakeoverSchema,{method:"POST",body:{},actionKey:newActionKey()}),enabled:canManage&&["preparing","awaiting_approval"].includes(item.state),refetchInterval:q=>q.state.data?.blockers?.length?5000:false,retry:false});
 const current=["completed","cancelled"].includes(item.state)?item:check.data||item;
 const blockers=current.blockers||[];
 return <Card><div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-xl font-semibold">{channelName}</h2><p className="mt-2 text-sm text-ink-mute"><When value={item.created_at}/></p></div><Status value={current.state} labels={{preparing:"正在准备",awaiting_approval:"待确认接管",completed:"接管完成",cancelled:"已取消"}}/></div>
 {notice&&<NoticeDetails notice={notice}/>}
 {item.evidence&&<p className="mt-3 text-sm">{["品牌确认承接渠道客户","经营方取消接管准备"].includes(item.evidence)?t(item.evidence):item.evidence}</p>}
 {blockers.length>0&&<div className="my-5"><Notice>{t("以下事项完成后可以接管；正常在途业务会自动重新检查。")}</Notice><ul className="mt-3 space-y-2 text-sm">{blockers.map((b,i)=><li key={i}>{t(b.label)}：{b.count}。{t("由原渠道在对应业务页面处理。")}</li>)}</ul></div>}
 {check.isFetching&&current.state==="preparing"&&!blockers.length&&<p className="my-5 text-sm">{t("正在检查在途业务…")}</p>}
 {!!check.error&&<ErrorPanel error={check.error} retry={()=>void check.refetch()}/>}
 {current.snapshot&&<TakeoverSummary item={current}/>}
 {current.state==="completed"&&<Notice>{current.completed_at&&<p>{t("接管完成时间")}：<When value={current.completed_at}/></p>}{t(canManage?"客户权益已承接，原渠道已结束经营。复制的套餐默认停止新购和续费，可在套餐页面决定是否开放。":"客户权益和套餐已由所属品牌承接，原渠道经营已结束。")}</Notice>}
 {canManage&&["preparing","awaiting_approval"].includes(current.state)&&<div className="mt-5 flex gap-3">{current.snapshot&&!blockers.length&&<Button disabled={check.isFetching||!!check.error} onClick={()=>setAction("execute")}>{t("确认接管")}</Button>}<Button variant="outline" onClick={()=>setAction("cancel")}>{t("取消接管")}</Button></div>}
 {action&&<TakeoverAction action={action} scope={scope} item={current} close={()=>setAction(undefined)}/>}
 </Card>;
}
function TakeoverSummary({item}:{item:Takeover}){
 const {t}=useI18n();const s=item.snapshot;if(!s)return null;
 return <div className="my-5 space-y-5"><div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><Stat label="承接客户" value={String(Object.keys(s.customers).length)}/><Stat label="客户剩余额度" value={s.service_atg} unit="ATG"/><Stat label="客户支付余额" value={s.wallet_money} unit="MONEY"/><Stat label="客户佣金" value={s.commission_money} unit="MONEY"/></div>
 <Notice><p>{t("现金责任合计")}：<Money value={s.cash_money} unit="MONEY"/>。{t("含可用和冻结佣金；已有占用不重复计入。双方在线下交接资金。")}</p><p>{t("渠道剩余采购余额")}：<Money value={s.procurement.quantity} unit="ATG"/>。{t("退出时核销并保留记录，不退款。")}</p></Notice>
 <h3 className="font-semibold">{t("随接管复制的套餐")}</h3>
 <p className="text-sm text-ink-mute">{t("标明来源渠道，默认停止新购和续费。已有订单的额度、赠送和期限继续有效；不限时额度仍不限时。")}</p>
 <Table rows={s.packages} getID={p=>p.id} columns={[{label:t("套餐"),cell:p=>p.name},{label:t("周期"),cell:p=>t(({month:"每月",quarter:"每季",year:"每年",lifetime:"不限时"} as Record<string,string>)[p.period]||p.period)},{label:t("售价"),cell:p=><Money value={p.price_money} unit="MONEY"/>},{label:t("购买额度"),cell:p=><Money value={p.paid_atg} unit="ATG"/>},{label:t("赠送额度"),cell:p=><Money value={p.gift_atg} unit="ATG"/>}]}/>
 </div>;
}
function TakeoverAction({action,scope,item,close}:{action:"execute"|"cancel";scope:string;item:Takeover;close:()=>void}){
 const {t}=useI18n();const command=useCommand(`${scope}/takeovers/${item.id}/${action}`,TakeoverSchema);
 async function submit(e:FormEvent<HTMLFormElement>){e.preventDefault();const input=action==="execute"?{expected_snapshot:item.snapshot_fingerprint}:{evidence:""};if(await command.run(input))close()}
 return <OperatorPanel open title={t(action==="execute"?"确认接管":"取消接管")} description={t("核对本次办理结果。")} blocked={command.busy||command.unresolved} close={close}>
 {action==="execute"&&<TakeoverSummary item={item}/>}<form onSubmit={submit} className="space-y-5"><fieldset disabled={command.busy||command.unresolved}><label className="block text-sm leading-7"><input required type="checkbox" className="mr-2"/>{t(action==="execute"?"确认完整承接客户权益，复制套餐并结束原渠道经营。双方资金交接在线下处理。":"确认取消接管；渠道营业由负责人检查后恢复。")}</label></fieldset>
 {!!command.error&&<ErrorPanel error={command.error}/>}<Button disabled={command.busy}>{command.unresolved?t("重试原操作"):t("确认提交")}</Button></form></OperatorPanel>;
}
