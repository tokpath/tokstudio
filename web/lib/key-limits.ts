export function parseAllowlist(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

/** 空字符串表示用服务端默认值；其它非正整数都是校验失败，不能静默丢掉。 */
export function optionalPositiveInt(raw: string): number | "empty" | "invalid" {
  const trimmed = raw.trim();
  if (!trimmed) {
    return "empty";
  }
  if (!/^[1-9]\d{0,5}$/.test(trimmed)) {
    return "invalid";
  }
  return Number(trimmed);
}
