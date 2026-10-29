"use client";
import { useI18n } from "@/components/i18n";
import { Decimal } from "@/lib/contracts";
import { compareDecimal, subtractDecimal } from "@/lib/decimal";
import { Money } from "./shared";

export function ReceiptAmounts({ gross, fee, confirmed, currency = "MONEY", original = false }: { gross: string; fee: string; confirmed: boolean; currency?: string; original?: boolean }) {
  const { t } = useI18n();
  const valid = (value: string) => Decimal.safeParse(value).success && (value.split(".")[1]?.length || 0) <= 18;
  const known = confirmed && valid(gross) && valid(fee);
  const exceeds = known && compareDecimal(fee, gross) > 0;
  const title = t(original ? "原币收款金额核对" : "收款金额核对");
  return <section aria-label={title} className="rounded-control border border-hairline bg-canvas p-4">
    <h3 className="mb-4 text-sm font-semibold">{title}</h3>
    <dl className="grid gap-4 text-sm sm:grid-cols-3">
      <div><dt className="mb-2 text-ink-secondary">{t("收款总额（扣手续费前）")}</dt><dd>{valid(gross) ? <Money value={gross} unit={currency} /> : "—"}</dd></div>
      <div><dt className="mb-2 text-ink-secondary">{t("己方收款手续费")}</dt><dd>{confirmed && valid(fee) ? <Money value={fee} unit={currency} /> : t(confirmed ? "待填写" : "待核实")}</dd></div>
      <div><dt className="mb-2 text-ink-secondary">{t("净到账金额")}</dt><dd className="font-semibold">{known && !exceeds ? <Money value={subtractDecimal(gross, fee)} unit={currency} /> : "—"}</dd></div>
    </dl>
    <p className={`mt-4 text-xs leading-6 ${exceeds ? "text-danger" : "text-ink-mute"}`}>{t(exceeds ? "己方收款手续费不能超过收款总额。" : !confirmed ? "手续费待核实，净到账金额暂不确定。" : original ? "原币净到账金额＝原币收款总额－原币手续费；订单交付按已核实结算金额计算。" : "净到账金额＝收款总额－己方收款手续费；订单交付仍按扣手续费前的收款总额计算。")}</p>
  </section>;
}
