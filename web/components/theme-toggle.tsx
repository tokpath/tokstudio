"use client";

import { useTheme } from "next-themes";
import { useTranslations } from "next-intl";
import { useEffect, useState } from "react";
import { THEME_PREFERENCES, type ThemePreference } from "@/lib/theme";

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  const t = useTranslations("theme");

  useEffect(() => {
    setMounted(true);
  }, []);

  const current = (mounted ? theme : "system") as string;
  const labels: Record<ThemePreference, string> = {
    light: t("light"),
    dark: t("dark"),
    system: t("system"),
  };

  return (
    <div className="inline-flex rounded-control border border-hairline bg-canvas-raised p-0.5" role="group" aria-label={t("label")}>
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
            {labels[item]}
          </button>
        );
      })}
    </div>
  );
}
