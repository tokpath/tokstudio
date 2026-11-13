"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Globe } from "lucide-react";
import { hasLocaleDictionary, installLocaleDictionary, localeCookie, localeNames, localeTags, locales, normalizeLocale, translate, type Locale, type LocaleDictionary, type Translator } from "@/lib/i18n";
import { useMonetary } from "./product/monetary";

type I18nContext = { locale: Locale; setLocale: (locale: Locale) => void; localeLoadFailed?: boolean };
const Context = createContext<I18nContext>({ locale: "zh", setLocale: () => {} });

export function I18nProvider({ initialLocale = "zh", dictionary, chineseDictionary, children }: {
  initialLocale?: Locale;
  dictionary?: LocaleDictionary;
  chineseDictionary?: LocaleDictionary;
  children: ReactNode;
}) {
  const [locale, updateLocale] = useState<Locale>(initialLocale);
  const [localeLoadFailed, setLocaleLoadFailed] = useState(false);
  const selection = useRef(0);
  if (chineseDictionary) installLocaleDictionary("zh", chineseDictionary);
  if (dictionary) installLocaleDictionary(initialLocale, dictionary);
  const setLocale = useCallback((next: Locale) => {
    if (!locales.includes(next)) return;
    const attempt = ++selection.current;
    setLocaleLoadFailed(false);
    // Persist the user's choice before a navigation can cancel the download.
    document.cookie = `${localeCookie}=${next}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`;
    function apply() {
      if (attempt !== selection.current) return;
      document.documentElement.lang = localeTags[next];
      updateLocale(next);
    }
    if (hasLocaleDictionary(next)) { apply(); return; }
    void fetch(`/locale/${next}`).then(async response => {
      if (!response.ok) throw new Error("Locale unavailable");
      installLocaleDictionary(next, await response.json() as LocaleDictionary);
      apply();
    }).catch(() => {
      if (attempt === selection.current) setLocaleLoadFailed(true);
    });
  }, []);
  useEffect(() => { document.documentElement.lang = localeTags[locale]; }, [locale]);
  const value = useMemo(() => ({ locale, setLocale, localeLoadFailed }), [locale, setLocale, localeLoadFailed]);
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useI18n() {
  const context = useContext(Context), monetary = useMonetary();
  const currency = monetary.status === "unconfigured" ? undefined : monetary.currency;
  const t = useCallback<Translator>((source, values) => translate(context.locale, source, values, currency), [context.locale, currency]);
  return { ...context, t };
}

export function LanguageControl() {
  const { locale, t, setLocale, localeLoadFailed } = useI18n();
  return <label className="flex min-h-10 shrink-0 items-center gap-1 text-ink-secondary"><Globe size={16} aria-hidden="true" /><select data-testid="language-selector" aria-label={t("显示语言")} value={locale} onChange={event => { const next = normalizeLocale(event.target.value); if (next) setLocale(next); }} className="min-h-10 max-w-36 rounded-control bg-transparent px-1 text-sm">{locales.map(item => <option key={item} value={item} lang={localeTags[item]}>{localeNames[item]}</option>)}</select>{localeLoadFailed && <span role="alert" className="text-xs text-danger">{t("语言加载失败，请重试。")}</span>}</label>;
}
