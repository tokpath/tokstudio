"use client";
import {useI18n} from "@/components/i18n";
import {Money} from "./shared";

export function CollectionFeeBreakdown({amount,fee,total,currency}:{amount:string;fee:string;total:string;currency:string}) {
 const {t}=useI18n();
 return <div className="space-y-2 rounded-control border border-hairline p-4 text-sm" aria-label={t("付款金额明细")}><div className="flex justify-between gap-3"><span>{t("订单或充值本金")}</span><Money value={amount} unit={currency}/></div><div className="flex justify-between gap-3"><span>{t("支付手续费")}</span><Money value={fee} unit={currency}/></div><div className="flex justify-between gap-3 border-t border-hairline pt-2 font-semibold"><span>{t("合计应付")}</span><Money value={total} unit={currency}/></div><p className="text-xs leading-6 text-ink-mute">{t("手续费不进入余额或 ATG，不计入分佣；余额和佣金抵扣部分不收取支付手续费。")}</p></div>;
}
