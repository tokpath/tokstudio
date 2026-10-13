import { NextIntlClientProvider } from "next-intl";
import type { ReactNode } from "react";
import zh from "@/messages/zh.json";
import en from "@/messages/en.json";
import ja from "@/messages/ja.json";

/** 单测里给 client 组件包一层中文消息，断言仍按现有中文文案。 */
export function withZh(ui: ReactNode) {
  return withLocale(ui, "zh");
}

export function withLocale(ui: ReactNode, locale: "zh" | "en" | "ja") {
  return (
    <NextIntlClientProvider locale={locale} messages={{zh, en, ja}[locale]}>
      {ui}
    </NextIntlClientProvider>
  );
}
