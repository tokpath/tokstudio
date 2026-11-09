import type { Translator } from "./i18n";

// Translate generated field labels, preserving every character of the supplied account details.
const labels = new Set(["收款人姓名", "办理说明", "开户银行", "银行卡号", "开户支行", "支付宝实名", "支付宝账号", "微信实名", "微信号或绑定手机号", "PayPal 邮箱", "账户姓名", "收款人英文姓名", "收款银行", "账号或 IBAN", "SWIFT/BIC", "银行所在国家或地区", "收款人地址", "收款银行地址", "补充汇款资料", "网络", "钱包地址"]);
export function payoutRecipientText(value: string, t: Translator): string {
  return value.split("\n").map(line => {
    const separator = line.indexOf("：");
    const label = line.slice(0, separator);
    return separator > 0 && labels.has(label) ? `${t(label)}: ${line.slice(separator + 1)}` : line;
  }).join("\n");
}
