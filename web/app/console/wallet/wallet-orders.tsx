"use client";
import { useLocale, useTranslations } from "next-intl";
import { usePersonalRecords } from "@/hooks/use-personal-records";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { formatPayMinor } from "@/lib/payment-quote";
import { formatUsdMinor } from "@/lib/money";
import { RecordPager } from "./wallet-ledger";
type Order={id:string;purpose:string;currency:string;amount_minor:number;credit_minor:number;status:string;created_at:string};
export function WalletOrders(){
 const t=useTranslations("walletExperience");const locale=useLocale();const list=usePersonalRecords<Order>("orders");
 return <section aria-label={t("ordersTitle")}><div className="mb-3 flex items-center justify-between gap-3"><h2 className="font-semibold">{t("ordersTitle")}</h2><Button variant="outline" disabled={list.refreshing} onClick={()=>void list.reload()}>{t("refresh")}</Button></div><ListResourceView snapshot={list.snapshot} emptyTitle={t("noOrders")} emptyDetail={t("noOrdersDetail")} onRetry={()=>void list.reload()}>{list.page && <><LedgerTable columns={[t("orderPurpose"),t("orderAmount"),t("credit"),t("status"),t("created"),t("orderId")]} emptyTitle={t("noOrders")} emptyDetail={t("noOrdersDetail")} rows={list.page.items.map(row=>({key:row.id,cells:[t.has(`purpose_${row.purpose}`)?t(`purpose_${row.purpose}`):t("purpose_other"),formatPayMinor(row.currency,row.amount_minor),formatUsdMinor(row.credit_minor),t.has(`status_${row.status}`)?t(`status_${row.status}`):t("status_unknown"),new Date(row.created_at).toLocaleString(locale),<span key="id" className="block max-w-48 whitespace-normal break-all font-mono">{row.id}</span>]}))}/><RecordPager total={list.page.total} cursor={list.cursor} next={list.page.next_cursor} href={list.href}/></>}</ListResourceView></section>;
}
