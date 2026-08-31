"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";

const COOKIE = "NEXT_LOCALE";

function readExplicitLocale() {
  if (typeof document === "undefined") {
    return "auto";
  }
  const match = document.cookie.match(/(?:^|; )NEXT_LOCALE=([^;]*)/);
  const value = match ? decodeURIComponent(match[1]) : "";
  if (value === "zh" || value === "en" || value === "ja") {
    return value;
  }
  return "auto";
}

/** 显式选择写 NEXT_LOCALE；自动则清 cookie，回退 Accept-Language。不改 URL。 */
export function LocaleSwitch() {
  const t = useTranslations("chrome");
  const [value, setValue] = useState("auto");

  useEffect(() => {
    setValue(readExplicitLocale());
  }, []);

  function choose(next: string) {
    if (next === "auto") {
      document.cookie = `${COOKIE}=; path=/; max-age=0`;
    } else {
      document.cookie = `${COOKIE}=${next}; path=/; max-age=31536000`;
    }
    window.location.reload();
  }

  return (
    <label className="inline-flex items-center gap-1 text-[13px] text-ink-mute">
      <span className="sr-only">{t("locale")}</span>
      <select
        aria-label={t("locale")}
        className="h-10 rounded-control border border-hairline bg-canvas px-2 text-sm text-ink"
        value={value}
        onChange={(event) => choose(event.target.value)}
      >
        <option value="auto">{t("localeAuto")}</option>
        <option value="zh">{t("localeZh")}</option>
        <option value="en">{t("localeEn")}</option>
        <option value="ja">{t("localeJa")}</option>
      </select>
    </label>
  );
}
