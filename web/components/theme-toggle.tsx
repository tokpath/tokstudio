"use client";

import { useTheme } from "next-themes";
import { useEffect, useState } from "react";
import { THEME_PREFERENCES, type ThemePreference } from "@/lib/theme";

const LABELS: Record<ThemePreference, string> = {
  light: "浅色",
  dark: "深色",
  system: "系统",
};

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  const current = (mounted ? theme : "system") as string;

  return (
    <div className="inline-flex rounded-control border border-hairline bg-canvas-raised p-0.5" role="group" aria-label="主题">
      {THEME_PREFERENCES.map((item) => {
        const active = current === item;
        return (
          <button
            key={item}
            type="button"
            onClick={() => setTheme(item)}
            className={`min-h-10 rounded-control px-2.5 text-[13px] max-sm:min-h-11 ${
              active ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute"
            }`}
          >
            {LABELS[item]}
          </button>
        );
      })}
    </div>
  );
}
