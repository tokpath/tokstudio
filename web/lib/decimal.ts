import { Decimal } from "./contracts";

export function formatDecimal(value: string, group = true): string {
  const negative = value.startsWith("-");
  const raw = negative ? value.slice(1) : value;
  if (!Decimal.safeParse(raw).success) return value;
  const [whole, fraction = ""] = raw.split(".");
  const digits = fraction.replace(/0+$/, "");
  const integer = group ? whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",") : whole;
  return `${negative ? "−" : ""}${integer}${digits ? `.${digits}` : ""}`;
}
// Presentation only: original strings remain the source of every asset decision.
export function previewDecimal(value:string,places=6):string {
  const negative=value.startsWith("-"),raw=negative?value.slice(1):value;
  if(!Decimal.safeParse(raw).success)return value;
  const [whole,fraction=""]=raw.split("."),tail=fraction.slice(places);
  if(!/[1-9]/.test(tail))return formatDecimal(value);
  const scaled=BigInt(whole+fraction.slice(0,places).padEnd(places,"0"));
  if(scaled===0n)return `${negative?"−":""}<0.${"0".repeat(places-1)}1`;
  const rounded=scaled+(tail[0]>="5"?1n:0n),digits=rounded.toString().padStart(places+1,"0");
  return "≈"+formatDecimal((negative?"-":"")+digits.slice(0,-places)+"."+digits.slice(-places));
}
export function compareDecimal(a: string, b: string): -1 | 0 | 1 {
  Decimal.parse(a); Decimal.parse(b);
  const scale = (value: string) => { const [whole, fraction = ""] = value.split("."); return BigInt(whole + fraction.padEnd(24, "0")); };
  const left = scale(a), right = scale(b);
  return left < right ? -1 : left > right ? 1 : 0;
}
export function safeAssetURL(value?: string | null): string | undefined {
  if (!value) return undefined;
  if (value.startsWith("/") && !value.startsWith("//")) return value;
  try { const parsed = new URL(value); if (parsed.protocol === "https:" || parsed.protocol === "http:") return parsed.toString(); } catch { /* Invalid URL stays unavailable. */ }
  return undefined;
}
