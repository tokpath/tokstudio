"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { useViewer } from "./viewer-context";
import { canAccessChannelPortal, canViewAdminHref } from "@/lib/rbac";
import { consoleHomeForViewer } from "@/lib/console-home";
import { loginHref, pagePathWithSearch } from "@/lib/login-next";

/** Do not mount protected page content until the session and portal are known. */
export function ConsoleAccess({ children }: { children: ReactNode }) {
  const viewer = useViewer();
  const pathname = usePathname();
  const t = useTranslations("common");
  const partner = pathname === "/partner" || pathname.startsWith("/partner/");
  const failed = viewer.error || (partner && viewer.partnerError);
  const allowed = viewer.signedIn && (
    pathname === "/admin" || pathname.startsWith("/admin/")
      ? canViewAdminHref(pathname, viewer)
      : pathname === "/channel" || pathname.startsWith("/channel/")
        ? canAccessChannelPortal(viewer.roles)
        : partner ? Boolean(viewer.isPartner) : true
  );
  if (!viewer.loading && !failed && allowed) return children;

  const next = pagePathWithSearch(pathname, typeof window === "undefined" ? "" : window.location.search);
  return (
    <main className="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-4 px-6" data-testid="console-access">
      <h1 className="text-2xl font-semibold" role={viewer.loading ? "status" : "alert"}>
        {viewer.loading ? t("listLoading") : failed ? t("listFailed") : !viewer.signedIn ? t("notLoggedIn") : t("listForbidden")}
      </h1>
      {!viewer.loading && (failed ? (
        <Button onClick={() => window.location.reload()}>{t("listRetry")}</Button>
      ) : !viewer.signedIn ? (
        <>
          <p>{t("listSessionExpiredDetail")}</p>
          <Button asChild><Link href={loginHref(next)}>{t("listRelogin")}</Link></Button>
        </>
      ) : (
        <Button asChild><Link href={consoleHomeForViewer(viewer)}>{t("returnConsole")}</Link></Button>
      ))}
    </main>
  );
}
