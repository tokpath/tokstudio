import type { Translator } from "./i18n";

// Generated labels may have an operator-authored specification suffix. Only
// system text is translated; the authored name stays literal in every locale.
export function billingLabel(value: string, t: Translator) {
  const parts = value.split(" · ");
  const authored = parts.length > 2 ? parts.pop() : undefined;
  const text = parts.map(part => part.split("、").map(name => t(name)).join("、")).join(" · ");
  return authored ? `${text} · ${authored}` : text;
}
