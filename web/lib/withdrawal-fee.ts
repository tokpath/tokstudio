import { Decimal } from "./contracts";

const scale = 18;

export type WithdrawalPreview =
  | { ok: true; amount: string; fee: string; net: string }
  | { ok: false; reason: "invalid" | "too_small" | "below_minimum" | "fee_exceeds" };

function atoms(value: string): bigint | undefined {
  if (!Decimal.safeParse(value).success) return undefined;
  const [whole, fraction = ""] = value.split(".");
  if (fraction.length > scale) return undefined;
  return BigInt(whole + fraction.padEnd(scale, "0"));
}

function text(value: bigint): string {
  const raw = value.toString().padStart(scale + 1, "0");
  const whole = raw.slice(0, -scale).replace(/^0+/, "") || "0";
  const fraction = raw.slice(-scale).replace(/0+$/, "");
  return fraction ? `${whole}.${fraction}` : whole;
}

// Preview matches the server: cut the requested amount down to the settlement
// currency's minor unit, then round the fee half up to that same unit.
export function previewWithdrawal(input: { amount: string; flatFee: string; feeRate: string; minimum: string; digits: number }): WithdrawalPreview {
  const amount = atoms(input.amount);
  const flat = atoms(input.flatFee);
  const rate = atoms(input.feeRate);
  const minimum = atoms(input.minimum);
  if (amount === undefined || flat === undefined || rate === undefined || minimum === undefined || input.digits < 0 || input.digits > 8) return { ok: false, reason: "invalid" };
  const unit = 10n ** BigInt(scale - input.digits);
  const payable = amount / unit * unit;
  if (payable <= 0n) return { ok: false, reason: "too_small" };
  if (payable < minimum) return { ok: false, reason: "below_minimum" };
  const exact = flat + payable * rate / (10n ** BigInt(scale));
  const remainder = exact % unit;
  const fee = remainder * 2n >= unit ? exact - remainder + unit : exact - remainder;
  const net = payable - fee;
  if (net <= 0n) return { ok: false, reason: "fee_exceeds" };
  return { ok: true, amount: text(payable), fee: text(fee), net: text(net) };
}
