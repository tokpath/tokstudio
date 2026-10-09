"use client";
import { useLocale, useTranslations } from "next-intl";
import Link from "next/link";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { usePersonalRecords } from "@/hooks/use-personal-records";
import { formatUsdMinor } from "@/lib/money";
type LedgerRow={id:string;event_type?:string;created_at?:string;amount_minor?:number};
export function RecordPager({total,cursor,next,href}:{total:number;cursor:string;next:string;href:(cursor:string)=>string}) {
 const t=useTranslations("walletExperience");
 return <div className="mt-4 flex flex-wrap items-center justify-between gap-3"><p className="text-sm text-ink-secondary">{t("total",{count:total})}</p><div className="flex gap-2">{cursor && <Button asChild variant="outline"><Link href={href("")}>{t("first")}</Link></Button>}{next && <Button asChild variant="outline"><Link href={href(next)}>{t("next")}</Link></Button>}</div></div>;
}
export function WalletLedger(){
 const t=useTranslations("user");const locale=useLocale();const tc=useTranslations("common");
 const events=new Set(["topup","authorization","release","usage_debit","refund","adjustment","commission_debit","commission_payout","gift_credit","commission_credit"]);
 const list=usePersonalRecords<LedgerRow>("ledger");
 return <section aria-label={t("ledTitle")}><div className="mb-3 flex items-center justify-between gap-3"><h2 className="font-semibold">{t("ledTitle")}</h2><Button variant="outline" size="sm" disabled={list.refreshing} onClick={()=>void list.reload()}>{tc("refresh")}</Button></div><ListResourceView name="ledger" snapshot={list.snapshot} loadingTitle={t("ledLoading")} emptyTitle={t("ledEmpty")} emptyDetail={t("ledEmptyDetail")} onRetry={()=>void list.reload()}>{list.page && <><LedgerTable columns={[t("ledColType"),t("ledColAmount"),t("ledColDate")]} emptyTitle={t("ledEmpty")} emptyDetail={t("ledEmptyDetail")} rows={list.page.items.map(row=>({key:row.id,cells:[t(`ledgerEvents.${events.has(row.event_type || "") ? row.event_type : "other"}`),<span key="amount" className="font-mono tabular-nums">{formatUsdMinor(row.amount_minor)}</span>,row.created_at ? new Date(row.created_at).toLocaleString(locale) : "—"]}))}/><RecordPager total={list.page.total} cursor={list.cursor} next={list.page.next_cursor} href={list.href}/></>}</ListResourceView></section>;
}
