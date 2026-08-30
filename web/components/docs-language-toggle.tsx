"use client";

import { useState } from "react";

const LOCALES = [
  { id: "zh", label: "中" },
  { id: "en", label: "EN" },
  { id: "ja", label: "日" },
] as const;

/** 三语共用这一套布局，不为某一语言换皮肤。语言包接通前只切字标。 */
export function DocsLanguageToggle() {
  const [locale, setLocale] = useState<(typeof LOCALES)[number]["id"]>("zh");

  return (
    <div className="flex flex-col gap-2">
      <div className="inline-flex rounded-control border border-hairline bg-canvas-raised p-0.5" role="group" aria-label="文档语言">
        {LOCALES.map((item) => {
          const active = locale === item.id;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => setLocale(item.id)}
              className={`min-h-10 rounded-control px-3 text-[13px] max-sm:min-h-11 ${
                active ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute"
              }`}
            >
              {item.label}
            </button>
          );
        })}
      </div>
      {locale !== "zh" ? (
        <p className="text-sm text-hold">三语文档共用这一套布局。语言包尚未接通，不另做皮肤。</p>
      ) : null}
    </div>
  );
}
