import { defaultCopy, type CopyTranslator } from "@/lib/copy";

const noticeKinds = ["balance_insufficient", "balance_low", "expiring_3d", "expiring_1d", "expired", "renewal_failed", "renewed", "quota_insufficient", "quota_low", "quota_recovered", "alert_opened", "alert_resolved", "smtp_test"];
const runbooks = ["rb_pending", "rb_circuit", "rb_backup", "rb_payment", "rb_media", "rb_tls"];

/** Only system-owned templates are translated. Names, IDs and operator-authored content stay intact. */
export function alertMessage(message: string, t: CopyTranslator): string {
  if (message === defaultCopy("systemAlerts.backup")) return t("systemAlerts.backup");
  if (message === defaultCopy("systemAlerts.pending")) return t("systemAlerts.pending");
  const prefix = defaultCopy("systemAlerts.lowSuccess", { provider: "" });
  if (message.startsWith(prefix)) return t("systemAlerts.lowSuccess", { provider: message.slice(prefix.length) });
  return message;
}

export function notificationCopy<T extends { kind: string; title: string; body: string }>(item: T, locale: string, t: CopyTranslator): T {
  if (locale === "zh" || !noticeKinds.includes(item.kind)) return item;
  const title = t(`noticeEvents.${item.kind}.title`);
  let body = t(`noticeEvents.${item.kind}.body`);
  if (item.kind.startsWith("alert_") && item.body.includes("\n")) {
    body += `\n${alertMessage(item.body.slice(item.body.indexOf("\n") + 1), t)}`;
  }
  return { ...item, title, body };
}

export function deliveryTitle(item: { kind?: string; title: string }, locale: string, t: CopyTranslator): string {
  if (locale === "zh") return item.title;
  const kind = item.kind || noticeKinds.find(kind => defaultCopy(`noticeEvents.${kind}.title`) === item.title);
  return kind && noticeKinds.includes(kind) ? t(`noticeEvents.${kind}.title`) : item.title;
}

export function runbookCopy<T extends { id: string; title: string; body: string }>(item: T, locale: string, t: CopyTranslator): T {
  if (locale === "zh" || !runbooks.includes(item.id)) return item;
  return { ...item, title: t(`systemRunbooks.${item.id}.title`), body: t(`systemRunbooks.${item.id}.body`) };
}
