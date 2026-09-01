"use client";

import { Check } from "lucide-react";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { cn } from "@/lib/utils";

export type ChromeIconMenuOption = {
  value: string;
  label: string;
  icon?: ReactNode;
};

/**
 * 顶栏用的「一个图标 + 弹出菜单」。
 * 主题、语言这类控件选项很多，但顶栏只能占一个图标的位置，所以把文字选项收进菜单里。
 */
export function ChromeIconMenu({
  label,
  icon,
  value,
  options,
  onChange,
}: {
  label: string;
  icon: ReactNode;
  value: string;
  options: ChromeIconMenuOption[];
  onChange: (value: string) => void;
}) {
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
    <div ref={rootRef} className="relative">
      <button
        type="button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        title={label}
        onClick={() => setOpen((next) => !next)}
        className="inline-flex size-10 items-center justify-center rounded-control text-ink-secondary transition-colors hover:bg-canvas-raised hover:text-ink max-sm:size-11"
      >
        {icon}
      </button>
      {open ? (
        <div
          id={menuId}
          role="menu"
          aria-label={label}
          className="absolute right-0 top-full z-50 mt-1.5 min-w-[11rem] rounded-card border border-hairline bg-canvas-raised p-1.5 shadow-[0_1px_2px_rgba(20,20,20,0.06)]"
        >
          {options.map((option) => {
            const active = option.value === value;
            return (
              <button
                key={option.value}
                type="button"
                role="menuitemradio"
                aria-checked={active}
                className={cn(
                  "flex w-full items-center gap-2 rounded-control px-2.5 py-2 text-left text-sm transition-colors duration-150",
                  active ? "bg-brand-soft text-brand-emphasis" : "text-ink hover:bg-canvas",
                )}
                onClick={() => {
                  setOpen(false);
                  if (!active) onChange(option.value);
                }}
              >
                {option.icon ? (
                  <span className="inline-flex size-4 shrink-0 items-center justify-center [&>svg]:size-4">
                    {option.icon}
                  </span>
                ) : null}
                <span className="flex-1">{option.label}</span>
                {active ? <Check className="size-3.5 shrink-0" aria-hidden /> : <span className="size-3.5 shrink-0" />}
              </button>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
