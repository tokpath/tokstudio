"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Bell } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { apiBase } from "@/lib/api";

export function notifyInboxChanged() { window.dispatchEvent(new Event("tokenhub:notifications-changed")); }

export function NotificationBell({ href }: { href: string }) {
  const t = useTranslations("notifications");
  const viewer = useViewer();
  const brand = useBrand();
  const [count, setCount] = useState<number | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setCount(null); setFailed(false);
    if (viewer.loading || !viewer.signedIn) return;
    let active = true; let seq = 0;
    const controller = new AbortController();
    async function refresh() {
      const request = ++seq;
      try {
        const response = await fetch(`${apiBase}/v1/me/notifications/unread-count`, { credentials: "include", signal: controller.signal });
        const body = await response.json();
        if (!response.ok || typeof body.unread !== "number") throw new Error();
        if (active && request === seq) { setCount(body.unread); setFailed(false); }
      } catch { if (active && request === seq) { setCount(null); setFailed(true); } }
    }
    const visible = () => { if (document.visibilityState === "visible") void refresh(); };
    const changed = () => void refresh();
    void refresh();
    const timer = setInterval(visible, 60_000);
    document.addEventListener("visibilitychange", visible);
    window.addEventListener("tokenhub:notifications-changed", changed);
    return () => { active = false; controller.abort(); clearInterval(timer); document.removeEventListener("visibilitychange", visible); window.removeEventListener("tokenhub:notifications-changed", changed); };
  }, [viewer.loading, viewer.signedIn, viewer.userId, brand?.id]);
  if (!viewer.signedIn) return null;
  const label = failed ? t("bellUnavailable") : count === null ? t("title") : t("bellLabel", { count });
  return <Button asChild variant="ghost" size="icon" className="relative shrink-0"><Link href={href} aria-label={label} title={label}>
    <Bell aria-hidden="true" />
    {count !== null && count > 0 && <span aria-hidden="true" className="absolute -right-1 top-0 min-w-4 rounded-full bg-brand px-1 text-center text-[10px] leading-4 text-white">{count > 99 ? "99+" : count}</span>}
  </Link></Button>;
}
