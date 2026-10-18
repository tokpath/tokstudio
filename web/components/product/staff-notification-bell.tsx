"use client";
import { useI18n } from "@/components/i18n";
import Link from "next/link";
import { Bell } from "lucide-react";
import { staffNotificationPath } from "@/lib/navigation";

export function StaffNotificationBell({ workspaces, activeOperatorID, pathname }: { workspaces: Parameters<typeof staffNotificationPath>[0]; activeOperatorID?: string; pathname: string }) {
  const { t } = useI18n();
  const href = staffNotificationPath(workspaces, pathname, activeOperatorID);
  if (!href) return null;
  const current = pathname === href || pathname.startsWith(`${href}/`);
  return <Link href={href} aria-label={t("通知管理")} aria-current={current ? "page" : undefined} className="inline-flex h-8 w-8 items-center justify-center rounded-control text-ink-secondary hover:bg-canvas hover:text-ink aria-[current=page]:bg-brand-soft aria-[current=page]:text-brand-emphasis"><Bell size={16} strokeWidth={1.75} aria-hidden="true" /></Link>;
}
