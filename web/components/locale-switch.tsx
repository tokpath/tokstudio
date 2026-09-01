"use client";

import { Globe } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { ChromeIconMenu } from "@/components/chrome-icon-menu";

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
    <ChromeIconMenu
      label={t("locale")}
      value={value}
      icon={<Globe className="size-[18px]" strokeWidth={1.75} />}
      options={[
        { value: "auto", label: t("localeAuto") },
        { value: "zh", label: t("localeZh") },
        { value: "en", label: t("localeEn") },
        { value: "ja", label: t("localeJa") },
      ]}
      onChange={choose}
    />
  );
}
