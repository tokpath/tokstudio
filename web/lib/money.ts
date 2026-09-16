/** 账本最小单位：1 USD = 1_000_000 minor（micro-USD）。只在展示/提交边界换算。 */
export const MICRO_PER_USD = 1_000_000;
const CENT_MICRO = 10_000;

export function parseUsdToMinor(raw: string): number | null {
  const text = raw.trim().replace(/^\$/, "");
  if (!text) {
    return null;
  }
  const match = text.match(/^([+-])?(\d+)(?:\.(\d+))?$/);
  if (!match) {
    return null;
  }
  const frac = match[3] ?? "";
  if (frac.length > 6) {
    return null;
  }
  const sign = match[1] === "-" ? -1 : 1;
  const whole = match[2].replace(/^0+(?=\d)/, "");
  const micros = (frac + "000000").slice(0, 6);
  const minor = Number(whole) * MICRO_PER_USD + Number(micros);
  if (!Number.isSafeInteger(minor)) {
    return null;
  }
  return sign * minor;
}

export function formatUsdMinor(minor: number | string | null | undefined, lessThanCent = ""): string {
  if (minor == null || (typeof minor === "string" && !minor.trim())) {
    return "—";
  }
  const value = typeof minor === "number" ? minor : Number(minor);
  if (!Number.isFinite(value)) {
    return "—";
  }
  const sign = value < 0 ? "-" : "";
  const abs = Math.trunc(Math.abs(value));
  if (abs === 0) {
    return `${sign}$0.00`;
  }
  if (abs < CENT_MICRO) {
    if (lessThanCent) {
      return `${sign}${lessThanCent}`;
    }
    const frac = abs.toString().padStart(6, "0").replace(/0+$/, "");
    return `${sign}$0.${frac}`;
  }
  const dollars = Math.floor(abs / MICRO_PER_USD);
  const micros = abs % MICRO_PER_USD;
  let cents = Math.round(micros / CENT_MICRO);
  let whole = dollars;
  if (cents === 100) {
    whole += 1;
    cents = 0;
  }
  return `${sign}$${whole}.${cents.toString().padStart(2, "0")}`;
}

export const USD_CREDIT = "usd_credit";
