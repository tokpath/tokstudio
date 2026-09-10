"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Bell, KeyRound, LayoutDashboard, UserRound } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import {
  avatarInitial,
  balancePillText,
  profileDash,
  readNailedAvailable,
  shellRole,
  type BalanceLoadState,
  type MeProfile,
} from "@/lib/user-shell";

type ShellMe = MeProfile & { id?: string };

export function UserShellBell() {
  const t = useTranslations("shell");
  return (
    <button
      type="button"
      disabled
      aria-disabled="true"
      title={t("bellSoon")}
      aria-label={t("bell")}
      data-testid="shell-bell"
      className="inline-flex size-10 items-center justify-center rounded-control text-ink-mute opacity-60 max-sm:size-11"
    >
      <Bell className="size-[18px]" strokeWidth={1.75} aria-hidden />
    </button>
  );
}

export function UserShellRightZone() {
  const t = useTranslations("shell");
  const router = useRouter();
  const [me, setMe] = useState<ShellMe | null>(null);
  const [meReady, setMeReady] = useState(false);
  const [balanceState, setBalanceState] = useState<BalanceLoadState>("loading");
  const [available, setAvailable] = useState<unknown>(undefined);
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const menuId = useId();

  useEffect(() => {
    let cancelled = false;
    async function loadMe() {
      try {
        const response = await fetch(`${apiBase}/v1/me`, { credentials: "include" });
        if (!response.ok) {
          if (!cancelled) {
            setMe(null);
            setMeReady(true);
          }
          return;
        }
        const body = (await response.json()) as { user?: ShellMe };
        if (!cancelled) {
          setMe(body.user ?? null);
          setMeReady(true);
        }
      } catch {
        if (!cancelled) {
          setMe(null);
          setMeReady(true);
        }
      }
    }
    async function loadBalance() {
      try {
        const response = await fetch(`${apiBase}/v1/me/balance`, { credentials: "include" });
        if (!response.ok) {
          if (!cancelled) {
            setBalanceState("error");
            setAvailable(undefined);
          }
          return;
        }
        const body = (await response.json()) as { balance?: { available?: string | number } };
        if (!cancelled) {
          setAvailable(readNailedAvailable(body));
          setBalanceState("ok");
        }
      } catch {
        if (!cancelled) {
          setBalanceState("error");
          setAvailable(undefined);
        }
      }
    }
    void loadMe();
    void loadBalance();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    function onDoc(event: MouseEvent) {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    }
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const name = profileDash(me?.display_name);
  const email = profileDash(me?.email);
  const role = shellRole(me?.roles);
  const roleLabel = role === "admin" ? t("roleAdmin") : t("roleUser");
  const initial = avatarInitial(me?.display_name, me?.email);
  const amount = balancePillText(balanceState, available);

  async function logout() {
    setOpen(false);
    try {
      await fetch(`${apiBase}/v1/auth/logout`, { method: "POST", credentials: "include" });
    } catch {
      // 仍然去登录页；会话清除失败也不假装还登着。
    }
    router.push("/login");
    router.refresh();
  }

  return (
    <>
      {balanceState === "loading" ? (
        <span
          data-testid="balance-pill"
          data-state="loading"
          aria-busy="true"
          aria-label={t("balanceLoading")}
          className="inline-flex h-9 min-w-[4.5rem] items-center rounded-control bg-success/15 px-3"
        >
          <span className="h-3 w-12 animate-pulse rounded-control bg-success/25" />
        </span>
      ) : (
        <Link
          href="/app/wallet"
          data-testid="balance-pill"
          data-state={balanceState}
          data-field="available"
          aria-label={t("balance")}
          className={`inline-flex h-9 items-center rounded-control px-3 font-mono text-sm font-medium tabular-nums no-underline ${
            balanceState === "error" || amount === "—"
              ? "bg-canvas-raised text-ink-mute"
              : "bg-success/15 text-success"
          }`}
        >
          {amount}
        </Link>
      )}

      <div ref={rootRef} className="relative">
        <button
          type="button"
          aria-haspopup="menu"
          aria-expanded={open}
          aria-controls={open ? menuId : undefined}
          aria-label={t("accountMenu")}
          data-testid="avatar-trigger"
          onClick={() => setOpen((next) => !next)}
          className="inline-flex max-w-[14rem] items-center gap-2 rounded-control px-1.5 py-1 text-left transition-colors hover:bg-canvas-raised max-sm:h-11"
        >
          <span
            aria-hidden
            className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-brand-soft font-medium text-sm text-brand-emphasis"
          >
            {meReady ? initial : "—"}
          </span>
          <span className="hidden min-w-0 flex-col leading-tight sm:flex">
            <span className="truncate text-sm text-ink">{meReady ? name : "—"}</span>
            <span className="truncate text-[12px] text-ink-mute">{roleLabel}</span>
          </span>
        </button>
        {open ? (
          <div
            id={menuId}
            role="menu"
            aria-label={t("accountMenu")}
            data-testid="avatar-menu"
            className="absolute right-0 top-full z-50 mt-1.5 w-[16.5rem] rounded-card border border-hairline bg-canvas-raised p-1.5 shadow-[0_1px_2px_rgba(20,20,20,0.06)]"
          >
            <div className="px-2.5 py-2">
              <p data-testid="menu-display-name" className="truncate text-sm font-medium text-ink">
                {name}
              </p>
              <p data-testid="menu-email" className="mt-0.5 truncate text-[12px] text-ink-mute">
                {email}
              </p>
            </div>
            <Link
              href="/app/profile"
              role="menuitem"
              className="flex w-full items-center gap-2 rounded-control px-2.5 py-2 text-sm text-ink no-underline hover:bg-canvas"
              onClick={() => setOpen(false)}
            >
              <UserRound className="size-4 shrink-0 text-ink-mute" strokeWidth={1.75} aria-hidden />
              {t("profile")}
            </Link>
            {me?.roles?.includes("platform_admin") ? (
              <Link
                href="/admin"
                role="menuitem"
                data-testid="menu-platform-admin"
                className="flex w-full items-center gap-2 rounded-control px-2.5 py-2 text-sm text-ink no-underline hover:bg-canvas"
                onClick={() => setOpen(false)}
              >
                <LayoutDashboard className="size-4 shrink-0 text-ink-mute" strokeWidth={1.75} aria-hidden />
                {t("platformAdmin")}
              </Link>
            ) : null}
            <Link
              href="/app/keys"
              role="menuitem"
              className="flex w-full items-center gap-2 rounded-control px-2.5 py-2 text-sm text-ink no-underline hover:bg-canvas"
              onClick={() => setOpen(false)}
            >
              <KeyRound className="size-4 shrink-0 text-ink-mute" strokeWidth={1.75} aria-hidden />
              {t("keys")}
            </Link>
            <div className="my-1.5 border-t border-hairline" role="separator" />
            <button
              type="button"
              role="menuitem"
              className="flex w-full items-center rounded-control px-2.5 py-2 text-left text-sm text-[color:var(--danger)] hover:bg-danger/10"
              onClick={() => void logout()}
            >
              {t("logout")}
            </button>
          </div>
        ) : null}
      </div>
    </>
  );
}
