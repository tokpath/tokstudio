import discoveryUx from "@/locales/discovery-ux.json";
import usabilityUx from "@/locales/usability-ux.json";
import operatorUx from "@/locales/operator-ux.json";
import customerUx from "@/locales/customer-ux.json";
import help from "@/locales/help.json";
import common from "@/locales/common.json";
import customer from "@/locales/customer.json";
import operator from "@/locales/operator.json";
import forms from "@/locales/forms.json";
import backend from "@/locales/backend.json";
import errors from "@/locales/errors.json";
import monetary from "@/locales/monetary.json";

import type { Locale, LocaleDictionary, Message } from "./i18n";

export const catalogs = { common, customer, operator, forms, backend, errors, help, customerUx, operatorUx, discoveryUx, usabilityUx, monetary };
export const messages: Record<string, Message> = Object.assign({}, ...Object.values(catalogs));

const dictionaries = new Map<Locale, LocaleDictionary>();

export function localeDictionary(locale: Locale): LocaleDictionary {
  const existing = dictionaries.get(locale);
  if (existing) return existing;
  const dictionary = Object.fromEntries(Object.entries(messages).flatMap(([source, message]) => {
    // Chinese is the source language. Send only overrides and text that
    // needs recognition for interpolation or settlement-currency display.
    if (locale === "zh" && (!message.zh || message.zh === source) && !source.includes("结算币种") && !/\{[A-Za-z_]\w*\}|%[sd]/.test(source)) return [];
    return [[source, message[locale] || source]];
  }));
  dictionaries.set(locale, dictionary);
  return dictionary;
}

export function catalogText(locale: Locale, source: string): string {
  return messages[source]?.[locale] || source;
}
