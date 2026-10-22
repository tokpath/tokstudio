import type { Translator } from "./i18n";

// Generated labels may have an operator-authored specification suffix. Only
// system text is translated; the authored name stays literal in every locale.
export function billingLabel(value: string, t: Translator) {
  const [prefix, ...suffix] = value.split(" · ");
  return [t(prefix), ...suffix.map(part => prefix === "缓存读取" ? t(part) : part)].join(" · ");
}
