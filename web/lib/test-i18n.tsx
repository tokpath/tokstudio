import { NextIntlClientProvider } from "next-intl";
import type { ReactNode } from "react";
import zh from "@/messages/zh.json";

/** 单测里给 client 组件包一层中文消息，断言仍按现有中文文案。 */
export function withZh(ui: ReactNode) {
  return (
    <NextIntlClientProvider locale="zh" messages={zh}>
      {ui}
    </NextIntlClientProvider>
  );
}
