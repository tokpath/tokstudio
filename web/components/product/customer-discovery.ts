import { Decimal } from "@/lib/contracts";

const precision = 24;
const scale = 10n ** BigInt(precision);
function atoms(value: string) {
  const [whole, fraction = ""] = Decimal.parse(value).split(".");
  return BigInt(whole + fraction.padEnd(precision, "0"));
}
function decimal(value: bigint, places = precision) {
  const digits = value.toString().padStart(places + 1, "0");
  const fraction = digits.slice(-places).replace(/0+$/, "");
  return digits.slice(0, -places) + (fraction ? `.${fraction}` : "");
}

// Shift the decimal point without converting financial input to Number.
export function percentage(value: string, toRate = false) {
  const amount = atoms(value);
  return toRate ? decimal(amount / 100n) : decimal(amount * 100n);
}

// A display preview only. The server rechecks terms, eligibility and budgets.
// Match ledger.Quantize: ATG is truncated toward zero at 18 decimal places.
export function concessionPreview(purchase: string, gift: string, points: string, maximum: string) {
  try {
    if (!/^(0|[1-9]\d*)(\.\d{1,16})?$/.test(points)) return undefined;
    const rate = atoms(percentage(points, true)), direct = atoms(maximum);
    if (rate <= 0n || rate > direct) return undefined;
    const base = atoms(purchase) + atoms(gift);
    const giftAtoms = base * rate / scale / (10n ** 6n);
    const extra = giftAtoms * (10n ** 6n);
    return { base: decimal(base), gift: decimal(giftAtoms, 18), total: decimal(base + extra), remainingPoints: percentage(decimal(direct - rate)), rate: percentage(points, true) };
  } catch { return undefined; }
}

export function qualificationProgress(confirmed: string, threshold: string) {
  const current = atoms(confirmed), target = atoms(threshold);
  return { remaining: decimal(target > current ? target - current : 0n), percent: target === 0n ? 100 : Number((current < target ? current : target) * 10000n / target) / 100 };
}

export function activityShareURL(inviteURL: string, planID: string, campaignID: string) {
  try {
    const url = new URL(inviteURL);
    if (!["http:", "https:"].includes(url.protocol) || !(url.searchParams.get("invite") || url.searchParams.get("invite_code"))) return undefined;
    // Preserve the server-issued host and attribution. Never use a visitor's
    // current domain or manufacture a new invitation relationship.
    url.pathname = "/plans";
    url.searchParams.set("plan", planID);
    url.searchParams.set("kol_activity", campaignID);
    url.hash = "";
    return url.toString();
  } catch { return undefined; }
}
