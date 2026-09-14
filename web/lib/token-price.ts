/** 管理台 Token 价按「美元 / 百万 token」填写；账本仍存美元/token。 */

const PER_TOKEN_LOOKS_LIKE = /^0\.0{4,}/;

export function perMillionToPerToken(raw: string): string {
  const t = raw.trim();
  if (t === "") {
    return "";
  }
  if (PER_TOKEN_LOOKS_LIKE.test(t)) {
    throw new Error("请填每百万 token 的美元，例如 2，不要填 0.000002");
  }
  const shifted = shiftDecimal(t, -6);
  if (shifted == null) {
    throw new Error(`无效单价 ${t}`);
  }
  return shifted;
}

export function perTokenToPerMillion(raw: string): string {
  const t = raw.trim();
  if (t === "") {
    return "";
  }
  const shifted = shiftDecimal(t, 6);
  if (shifted == null) {
    return t;
  }
  return shifted;
}

export function formatIOPerMillion(pair?: string): string {
  if (!pair) {
    return "";
  }
  return pair
    .split("/")
    .map((part) => perTokenToPerMillion(part))
    .join("/");
}

export function millionDim(input: string, output: string): Record<string, string> | undefined {
  const next: Record<string, string> = {};
  const inTok = perMillionToPerToken(input);
  const outTok = perMillionToPerToken(output);
  if (inTok) {
    next.input = inTok;
  }
  if (outTok) {
    next.output = outTok;
  }
  return Object.keys(next).length > 0 ? next : undefined;
}

function shiftDecimal(raw: string, powerOfTen: number): string | null {
  const m = raw.trim().match(/^([+-])?(\d+)(?:\.(\d+))?$/);
  if (!m) {
    return null;
  }
  const sign = m[1] === "-" ? "-" : "";
  let digits = `${m[2]}${m[3] ?? ""}`.replace(/^0+(?=\d)/, "");
  let frac = m[3]?.length ?? 0;
  frac -= powerOfTen;
  if (digits === "0" && frac <= 0) {
    return "0";
  }
  if (frac > 0) {
    if (digits.length <= frac) {
      digits = digits.padStart(frac + 1, "0");
    }
    const cut = digits.length - frac;
    let out = `${digits.slice(0, cut)}.${digits.slice(cut)}`;
    out = out.replace(/\.?0+$/, "");
    return `${sign}${out}`;
  }
  if (frac < 0) {
    digits = digits.padEnd(digits.length - frac, "0");
    return `${sign}${digits.replace(/^0+(?=\d)/, "")}`;
  }
  return `${sign}${digits}`;
}
