"use client";

import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import { THEME_PREFERENCES, type ThemePreference, isThemePreference } from "@/lib/theme";

const labels: Record<ThemePreference, string> = {
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

  const current: ThemePreference = isThemePreference(theme) ? theme : "system";

  return (
    <div
      className="inline-flex rounded-control border border-hairline bg-canvas-raised p-0.5"
      role="group"
      aria-label="外观"
    >
      {THEME_PREFERENCES.map((value) => {
        const active = mounted && current === value;
        return (
          <button
            key={value}
            type="button"
            onClick={() => setTheme(value)}
            className={`min-h-9 rounded-control px-2.5 text-xs font-medium transition-colors ${
              active ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute hover:text-ink"
            }`}
            aria-pressed={active}
          >
            {labels[value]}
          </button>
        );
      })}
    </div>
  );
}
