"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { NextIntlClientProvider } from "next-intl";
import { Globe } from "lucide-react";
import { localeCookie, localeNames, localeTags, locales, normalizeLocale, translate, type Locale, type Translator } from "@/lib/i18n";
import { useMonetary } from "./product/monetary";

type I18nContext = { locale: Locale; t: Translator; setLocale: (locale: Locale) => void };
const Context = createContext<I18nContext>({ locale: "zh", t: (source, values) => translate("zh", source, values), setLocale: () => {} });

export function I18nProvider({ initialLocale = "zh", children }: { initialLocale?: Locale; children: ReactNode }) {
  const [locale, updateLocale] = useState<Locale>(initialLocale);
  const setLocale = useCallback((next: Locale) => {
    if (!locales.includes(next)) return;
    document.cookie = `${localeCookie}=${next}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`;
    document.documentElement.lang = localeTags[next];
    updateLocale(next);
  }, []);
  useEffect(() => { document.documentElement.lang = localeTags[locale]; }, [locale]);
  const value = useMemo(() => ({ locale, setLocale, t: ((source, values) => translate(locale, source, values)) as Translator }), [locale, setLocale]);
  return <Context.Provider value={value}><NextIntlClientProvider locale={localeTags[locale]} messages={{}} timeZone="Asia/Shanghai">{children}</NextIntlClientProvider></Context.Provider>;
}

export function useI18n() {
  const context = useContext(Context), monetary = useMonetary();
  const currency = monetary.status === "unconfigured" ? undefined : monetary.currency;
  const t = useCallback<Translator>((source, values) => translate(context.locale, source, values, currency), [context.locale, currency]);
  return { ...context, t };
}

export function LanguageControl() {
  const { locale, t, setLocale } = useI18n();
  return <label className="flex min-h-10 shrink-0 items-center gap-1 text-ink-secondary"><Globe size={16} aria-hidden="true" /><select data-testid="language-selector" aria-label={t("显示语言")} value={locale} onChange={event => { const next = normalizeLocale(event.target.value); if (next) setLocale(next); }} className="min-h-10 max-w-36 rounded-control bg-transparent px-1 text-sm">{locales.map(item => <option key={item} value={item} lang={localeTags[item]}>{localeNames[item]}</option>)}</select></label>;
}
