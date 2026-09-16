"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { AlertCircle } from "lucide-react";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Button } from "@/components/ui/button";
import { loginHref, pagePathWithSearch } from "@/lib/login-next";
import type { ListSnapshot } from "@/lib/list-resource";

export function ListResourceView<T>({
  snapshot,
  emptyTitle,
  emptyDetail,
  emptyAction,
  loadingTitle,
  onRetry,
  name,
  passEmpty = false,
  children,
}: {
  snapshot: ListSnapshot<T>;
  emptyTitle: string;
  emptyDetail: string;
  emptyAction?: ReactNode;
  loadingTitle?: string;
  onRetry: () => void;
  name?: string;
  passEmpty?: boolean;
  children: ReactNode;
}) {
  const tc = useTranslations("common");
  const pathname = usePathname();
  const reason = snapshot.message || (snapshot.httpStatus ? `${tc("listFailed")} (${snapshot.httpStatus})` : tc("listNetwork"));
  const testId = name ? `list-resource-${name}` : "list-resource";
  const returnPath = pagePathWithSearch(pathname, typeof window === "undefined" ? "" : window.location.search);

  if (snapshot.phase === "loading") {
    return (
      <div
        className="mt-3 rounded-card border border-hairline bg-canvas-raised px-8 py-12"
        data-testid={testId}
        data-list-phase="loading"
        aria-busy="true"
        aria-live="polite"
      >
        <div className="space-y-2" aria-hidden>
          <div className="h-4 w-36 animate-pulse rounded-md bg-brand-soft" />
          <div className="h-4 w-full max-w-xl animate-pulse rounded-md bg-brand-soft" />
          <div className="h-4 w-2/3 max-w-lg animate-pulse rounded-md bg-brand-soft" />
        </div>
        <p className="mt-4 text-sm font-medium text-ink">{loadingTitle || tc("listLoading")}</p>
        <p className="mt-2 text-sm text-ink-mute">{tc("listLoadingDetail")}</p>
      </div>
    );
  }

  if (snapshot.phase === "empty" && !passEmpty) {
    return (
      <div className="mt-3" data-testid={testId} data-list-phase="empty">
        <EmptyLedger title={emptyTitle} detail={emptyDetail} action={emptyAction} />
      </div>
    );
  }

  if (snapshot.phase === "error") {
    return (
      <div className="mt-3" data-testid={testId} data-list-phase="error" role="alert">
        <EmptyLedger
          icon={AlertCircle}
          title={tc("listFailed")}
          detail={reason}
          action={
            <Button type="button" variant="outline" onClick={onRetry}>
              {tc("listRetry")}
            </Button>
          }
        />
      </div>
    );
  }

  if (snapshot.phase === "unauthorized") {
    const forbidden = snapshot.auth === "forbidden";
    return (
      <div className="mt-3" data-testid={testId} data-list-phase="unauthorized" role="alert">
        <EmptyLedger
          icon={AlertCircle}
          title={forbidden ? tc("listForbidden") : tc("listSessionExpired")}
          detail={forbidden ? reason || tc("listForbidden") : tc("listSessionExpiredDetail")}
          action={
            forbidden ? (
              <Button type="button" variant="outline" onClick={onRetry}>
                {tc("listRetry")}
              </Button>
            ) : (
              <Button asChild>
                <Link href={loginHref(returnPath)}>{tc("listRelogin")}</Link>
              </Button>
            )
          }
        />
      </div>
    );
  }

  return (
    <div data-testid={testId} data-list-phase={snapshot.phase}>
      {snapshot.phase === "stale" ? (
        <p className="mb-3 rounded-stamp border border-hairline bg-canvas px-3 py-2 text-sm text-ink-secondary" role="alert">
          {tc("listUpdateFailed")} {reason}
          <Button type="button" size="sm" variant="outline" className="ml-3" onClick={onRetry}>
            {tc("listRetry")}
          </Button>
        </p>
      ) : null}
      {children}
    </div>
  );
}
