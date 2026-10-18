import { translate, type Locale } from "@/lib/i18n";
import { Decimal } from "@/lib/contracts";

// Funding suggestions only distribute a server quote. They never calculate a
// price, grant, eligibility threshold, or whether an order has been paid.
const precision = 24;
function units(value: string) {
  const [whole, fraction = ""] = Decimal.parse(value).split(".");
  return BigInt(whole + fraction.padEnd(precision, "0"));
}
function decimal(value: bigint) {
  const digits = value.toString().padStart(precision + 1, "0");
  return `${digits.slice(0, -precision)}.${digits.slice(-precision)}`.replace(/\.?0+$/, "");
}
// Display the grant total without rounding high precision ATG amounts.
export function totalCredits(...amounts: string[]) {
  return decimal(amounts.reduce((total, amount) => total + units(amount), 0n));
}
export function fundingSuggestion(amount: string, availableBalance: string, availableCommission = "0", useCommission = false) {
  const total = units(amount), balance = units(availableBalance), commission = useCommission ? units(availableCommission) : 0n;
  const usdBalance = total < balance ? total : balance;
  const remaining = total - usdBalance;
  const commissionUSD = remaining < commission ? remaining : commission;
  return { balance: decimal(usdBalance), commission: decimal(commissionUSD), external: decimal(remaining - commissionUSD) };
}
export function fundingPreview(amount: string, balance: string, commission: string, availableBalance?: string, availableCommission?: string, locale: Locale = "zh") {
  const t = (source: string) => translate(locale, source);
  try {
    const total = units(amount), b = units(balance), c = units(commission);
    if (availableBalance !== undefined && b > units(availableBalance)) return { error: t("所选 USD 支付余额超过当前可用金额。") };
    if (availableCommission !== undefined && c > units(availableCommission)) return { error: t("所选佣金超过当前可用金额。") };
    if (b + c > total) return { error: t("支付来源合计超过本次应付，请调整分配。") };
    return { external: decimal(total - b - c) };
  } catch { return { error: t("请输入非负十进制 USD 金额，最多 24 位小数。") }; }
}
export const planPeriods: Record<string, string> = { unlimited: "不限时", month: "一个日历月", quarter: "三个日历月", year: "十二个日历月" };
export const budgetPeriods = { lifetime: "不限时累计", month: "自然月", quarter: "自然季", year: "自然年" };
