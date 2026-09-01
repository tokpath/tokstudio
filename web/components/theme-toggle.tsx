"use client";

import { Monitor, Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useTranslations } from "next-intl";
import { useEffect, useState } from "react";
import { ChromeIconMenu } from "@/components/chrome-icon-menu";
import { THEME_PREFERENCES, type ThemePreference } from "@/lib/theme";

const THEME_ICONS = {
  light: Sun,
  dark: Moon,
  system: Monitor,
} as const;

export function ThemeToggle() {
  const { theme, setTheme, resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  const t = useTranslations("theme");

  useEffect(() => {
    setMounted(true);
  }, []);

  const preference = (mounted && theme ? theme : "system") as ThemePreference;
  const TriggerIcon = mounted && resolvedTheme === "dark" ? Moon : Sun;

  return (
    <ChromeIconMenu
      label={t("label")}
      value={preference}
      icon={<TriggerIcon className="size-[18px]" strokeWidth={1.75} />}
      options={THEME_PREFERENCES.map((item) => {
        const Icon = THEME_ICONS[item];
        return {
          value: item,
          label: t(item),
          icon: <Icon strokeWidth={1.75} />,
        };
      })}
      onChange={setTheme}
    />
  );
}
