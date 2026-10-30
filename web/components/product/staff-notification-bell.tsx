"use client";
import { useI18n } from "@/components/i18n";
import Link from "next/link";
import { Bell } from "lucide-react";
import { staffNotificationPath } from "@/lib/navigation";
import { UnreadBadge, useUnreadNotifications } from "./notification-count";

export function StaffNotificationBell({ workspaces, activeOperatorID, pathname }: { workspaces: Parameters<typeof staffNotificationPath>[0]; activeOperatorID?: string; pathname: string }) {
  const { t } = useI18n();
  const href = staffNotificationPath(workspaces, pathname, activeOperatorID);
  const operatorID = href?.split("/")[2];
  const count = useUnreadNotifications(operatorID ? `/api/v1/operators/${operatorID}/notifications/unread-count` : undefined);
  if (!href) return null;
  const current = pathname === href || pathname.startsWith(`${href}/`);
  const label = count > 0 ? t("通知管理，{count} 条未读", { count }) : t("通知管理");
  return <Link href={href} aria-label={label} aria-current={current ? "page" : undefined} className="relative inline-flex h-8 w-8 items-center justify-center rounded-control text-ink-secondary hover:bg-canvas hover:text-ink aria-[current=page]:bg-brand-soft aria-[current=page]:text-brand-emphasis"><Bell size={16} strokeWidth={1.75} aria-hidden="true" /><UnreadBadge count={count} /></Link>;
}
