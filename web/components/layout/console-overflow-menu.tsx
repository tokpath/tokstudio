"use client";

import { Bell, EllipsisVertical, Search } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { LocaleSwitch } from "@/components/locale-switch";
import { ThemeToggle } from "@/components/theme-toggle";

export function ConsoleOverflowMenu({
  onCommand,
  showBell = false,
}: {
  onCommand: () => void;
  showBell?: boolean;
}) {
  const tc = useTranslations("chrome");
  const ts = useTranslations("shell");
  const tt = useTranslations("theme");
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const menuId = useId();

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

  return (
    <div ref={rootRef} className="relative md:hidden">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={tc("moreTools")}
        data-testid="chrome-overflow-trigger"
        title={tc("moreTools")}
        onClick={() => setOpen((next) => !next)}
        className="inline-flex size-10 shrink-0 items-center justify-center rounded-control text-ink-secondary transition-colors hover:bg-canvas-raised hover:text-ink max-sm:size-11"
      >
        <EllipsisVertical className="size-[18px]" strokeWidth={1.75} aria-hidden />
      </button>
      {open ? (
        <div
          id={menuId}
          role="menu"
          aria-label={tc("moreTools")}
          data-testid="chrome-overflow-menu"
          className="absolute right-0 top-full z-50 mt-1.5 w-[min(18rem,calc(100vw-1.5rem))] rounded-card border border-hairline bg-canvas-raised p-1.5 shadow-[0_1px_2px_rgba(20,20,20,0.06)]"
        >
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 rounded-control px-2.5 py-2 text-left text-sm text-ink hover:bg-canvas"
            onClick={() => {
              setOpen(false);
              onCommand();
            }}
          >
            <Search className="size-4 shrink-0 text-ink-mute" strokeWidth={1.75} aria-hidden />
            {tc("jump")}
          </button>
          {showBell ? (
            <button
              type="button"
              role="menuitem"
              disabled
              aria-disabled="true"
              title={ts("bellSoon")}
              className="flex w-full items-center gap-2 rounded-control px-2.5 py-2 text-left text-sm text-ink-mute"
            >
              <Bell className="size-4 shrink-0" strokeWidth={1.75} aria-hidden />
              {ts("bell")}
            </button>
          ) : null}
          <div className="my-1.5 border-t border-hairline" role="separator" />
          <div className="flex items-center justify-between gap-2 px-1.5 py-1">
            <span className="text-sm text-ink-secondary">{tc("locale")}</span>
            <LocaleSwitch />
          </div>
          <div className="flex items-center justify-between gap-2 px-1.5 py-1">
            <span className="text-sm text-ink-secondary">{tt("label")}</span>
            <ThemeToggle />
          </div>
        </div>
      ) : null}
    </div>
  );
}
