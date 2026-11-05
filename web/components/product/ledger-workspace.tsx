"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import type { Entity, JSONValue, ResourcePage } from "@/lib/contracts";
import { Heading, Money, Table, When } from "./shared";
import { OperatorPanel, ReadFacts } from "./operator-resources";

export const ledgerEventLabels: Record<string, string> = {
 external_receipt:"外部收款入账", receipt_allocation:"收款分配到订单", wallet_topup:"余额充值入账", topup:"余额充值入账", package_payment:"套餐付款", grant:"权益发放", usage:"模型调用扣费", expiry:"权益到期", rights_cancellation:"权益收回", usage_reversal:"调用扣费冲正", commission_accrual:"佣金计提", commission_release:"佣金解冻", commission_recovery:"佣金待抵扣结清", commission_correction:"佣金冲正", procurement:"采购额度入账", wholesale_sale:"供货交付记账", procurement_usage_reversal:"采购扣费冲正", procurement_removal:"采购退出额度收回", merchant_fee:"收款手续费记账", refund_merchant_fee:"退款手续费记账", procurement_refund_fee:"采购退款手续费记账", provider_settlement:"通道结算到账", external_refund:"退款付款记账", package_nondelivery_refund:"未交付套餐退款", procurement_nondelivery_refund:"未交付采购退款", external_chargeback:"外部扣回记账", chargeback_recovery:"扣回款项追偿", invalid_payment_assets:"无效付款权益处理", customer_responsibility_in:"接管客户权益转入", customer_responsibility_out:"接管客户权益转出", takeover_cash_settlement:"接管资金结算", takeover_counterparty_settlement:"接管双方结清", procurement_exit_obligation:"采购退出应收应付确认", procurement_exit_refund:"采购退出退款付款", procurement_exit_refund_received:"采购退出退款收款", supplier_cost:"供应商调用成本", correction:"账务冲正", commission_payout:"佣金提现付款", balance_payout:"余额提现付款", commission_withdrawal:"佣金提现付款", balance_withdrawal:"余额提现付款",
};
type Source = {kind: string; name: string; at?: string; url?: string; amount?: string; unit?: string};
type Display = {owner: string; purpose: string; auxiliary: boolean; source?: Source; effective_at?: string; expires_at?: string; availability?: string; available?: string};
type Entry = {account_id: string; owner_id: string; purpose: string; amount: string; unit: string; balance_after: string; display: Display};
const display = (row: Entity) => row.values.display as unknown as Display;
const entries = (row: Entity) => (row.values.entries || []) as unknown as Entry[];

function SourceInfo({ source }: {source?: Source}) {
 const {t}=useI18n();
 if (!source) return null;
 return <div className="space-y-1 text-xs leading-5 text-ink-secondary">
  <p>{t(source.kind)}{source.name && <> · {source.name}</>}{source.amount && <> · <Money value={source.amount} unit={source.unit} /></>}</p>
  {source.at && <p><When value={source.at} /></p>}
  {source.url && <a className="inline-block py-1 text-brand-emphasis underline" href={source.url}>{t("查看原业务")}</a>}
 </div>;
}
function AccountName({ value, source = true }: {value: Display; source?: boolean}) {
 const {t}=useI18n();
 return <div className="min-w-40 max-w-80 space-y-1 break-words"><p className="font-medium">{value.owner || t(value.source?.kind || "未记录所属对象")}</p><p className="text-xs text-ink-secondary">{t(value.purpose)}{value.auxiliary && <span className="ml-2 text-ink-mute">{t("记账辅助")}</span>}</p>{source && <SourceInfo source={value.source} />}</div>;
}
function AmountChange({value,unit}:{value:string;unit:string}) {return <span className="whitespace-nowrap">{!value.startsWith("-") && !/^0(?:\.0+)?$/.test(value) && "+"}<Money value={value} unit={unit} /></span>;}
function EntryTable({ row }: {row: Entity}) {
 return <Table rows={entries(row)} getID={entry=>entry.account_id} columns={[
  {label:"账户与业务来源",cell:entry=><AccountName value={entry.display} />},
  {label:"本次变动",align:"right",cell:entry=><AmountChange value={entry.amount} unit={entry.unit} />},
  {label:"本次入账后余额",align:"right",cell:entry=><Money value={entry.balance_after} unit={entry.unit} />},
 ]} />;
}
function TechnicalRecord({row}:{row:Entity}) {
 const {t}=useI18n();const [copied,setCopied]=useState(false);
 const raw={...row.values};delete raw.display;delete raw.source;
 if (Array.isArray(raw.entries)) raw.entries=raw.entries.map(entry=>{const v={...(entry as Record<string,JSONValue>)};delete v.display;return v;});
 return <details className="mt-7 border-t border-hairline pt-4"><summary className="cursor-pointer text-sm text-ink-secondary">{t("原始记录与编号")}</summary><div className="mt-4 space-y-4"><div className="flex flex-wrap items-center gap-2 text-xs"><code className="break-all">{row.id}</code><Button variant="outline" size="sm" onClick={async()=>{try {await navigator.clipboard.writeText(row.id);setCopied(true);} catch {setCopied(false);}}}>{t(copied ? "已复制" : "复制编号")}</Button></div><ReadFacts value={raw} /></div></details>;
}
export function LedgerCollection({page,view,toolbar}:{page:ResourcePage;view:string;toolbar?:ReactNode}) {
 const {t}=useI18n(); const [selected,setSelected]=useState<Entity>();
 useEffect(()=>setSelected(undefined),[view,page]);
 const journals=view==="journals";
 const event=(row:Entity)=>t(ledgerEventLabels[String(row.values.kind)] || "其他账务事项");
 const participants=(row:Entity)=>[...new Set(entries(row).filter(e=>!e.display.auxiliary).map(e=>e.display.owner).filter(Boolean))];
 const journalParties=(row:Entity)=>{const owners=participants(row);return owners.length ? owners : [...new Set(entries(row).map(e=>e.display.owner).filter(Boolean))];};
 const inspect=(row:Entity)=><Button size="sm" variant="outline" onClick={()=>setSelected(row)}>{t("查看详情")}</Button>;
 const amount=(row:Entity,key:string)=><Money value={String(row.values[key])} unit={String(row.values.unit)} />;
 const accounts=(rows:Entity[])=><Table rows={rows} getID={row=>row.id} empty="当前筛选下没有资产账户。" columns={[
  {label:"所属对象",cell:row=><p className="min-w-40 max-w-72 break-words font-medium">{display(row).owner || t(display(row).source?.kind || "未记录所属对象")}</p>},
  {label:"资产类型与来源",cell:row=><div className="min-w-48 max-w-80 space-y-2"><p>{t(display(row).purpose)}{display(row).auxiliary && <span className="ml-2 text-xs text-ink-mute">{t("记账辅助")}</span>}</p><SourceInfo source={display(row).source} />{display(row).effective_at && <p className="text-xs text-ink-secondary">{t("生效时间")} · <When value={display(row).effective_at} /></p>}{String(row.values.purpose).startsWith("customer:atg:") && <p className="text-xs text-ink-secondary">{display(row).expires_at ? <>{t("到期时间")} · <When value={display(row).expires_at} /></> : t("不限时")}</p>}</div>},
  {label:"账面余额",align:"right",cell:row=>amount(row,"balance")},
  {label:"占用",align:"right",cell:row=>amount(row,"held")},
  {label:"可用",align:"right",cell:row=><div>{display(row).available!==undefined ? <Money value={display(row).available!} unit={String(row.values.unit)} /> : <span className="text-ink-mute">—</span>}{display(row).availability && <p className="mt-1 text-xs text-ink-secondary">{t(display(row).availability!)}</p>}</div>},
  {label:"详情",cell:inspect},
 ]} />;
 return <>
  <Heading title={t("资产与账本")} description={t(journals ? "按入账时间查看业务引起的账户变化。金额与入账后余额保留原币种和 ATG 单位。" : "查看客户与经营方的余额、占用和可用资产。订单、收款和内部记账账户可按需一并查看。")} />
  {toolbar && <div className="th-toolbar">{toolbar}</div>}
  {journals ? <Table rows={page.items} getID={row=>row.id} empty="当前没有记账流水。" columns={[
   {label:"入账时间",cell:row=><div className="min-w-32"><When value={String(row.values.created_at)} /></div>},
   {label:"业务事项",cell:row=><div className="min-w-40 space-y-2"><p className="font-medium">{event(row)}</p><SourceInfo source={row.values.source as unknown as Source | undefined} /></div>},
   {label:"相关客户或经营方",cell:row=><div className="min-w-40 max-w-72 space-y-1">{journalParties(row).map(owner=><p key={owner}>{owner}</p>)}</div>},
   {label:"变动摘要",cell:row=><div className="min-w-52 max-w-80 space-y-2">{(entries(row).some(e=>!e.display.auxiliary) ? entries(row).filter(e=>!e.display.auxiliary) : entries(row)).map(e=><p key={e.account_id} className="flex flex-wrap justify-between gap-x-3 text-xs"><span>{t(e.display.purpose)}</span><AmountChange value={e.amount} unit={e.unit} /></p>)}</div>},
   {label:"详情",cell:inspect},
  ]} /> : accounts(page.items)}
  {selected && <OperatorPanel open title={journals ? event(selected) : t(display(selected).purpose)} description={journals ? t("核对本次业务的账户变动和原始依据。") : display(selected).owner || t(display(selected).source?.kind || "未记录所属对象")} close={()=>setSelected(undefined)}>
   {journals ? <div className="space-y-5"><p className="text-sm text-ink-secondary">{t("入账时间")} · <When value={String(selected.values.created_at)} /></p><SourceInfo source={selected.values.source as unknown as Source | undefined} /><EntryTable row={selected} /></div> : <div className="space-y-5">
    <SourceInfo source={display(selected).source} />
    <dl className="grid grid-cols-1 gap-4 sm:grid-cols-3">
     {["balance", "held", "available"].map((key,index)=><div key={key}><dt className="mb-1 text-sm text-ink-secondary">{t(["账面余额", "占用", "可用"][index])}</dt><dd>{key !== "available" ? amount(selected,key) : display(selected).available !== undefined ? <Money value={display(selected).available!} unit={String(selected.values.unit)} /> : "—"}</dd></div>)}
    </dl>
    {display(selected).availability && <p className="text-sm text-ink-secondary">{t(display(selected).availability!)}</p>}
    {display(selected).effective_at && <p className="text-sm">{t("生效时间")} · <When value={display(selected).effective_at} /></p>}
    {String(selected.values.purpose).startsWith("customer:atg:") && <p className="text-sm">{display(selected).expires_at ? <>{t("到期时间")} · <When value={display(selected).expires_at} /></> : t("不限时")}</p>}
   </div>}
   <TechnicalRecord row={selected} />
  </OperatorPanel>}
 </>;
}
