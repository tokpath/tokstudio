import type { Theme } from "./contracts";
export function resolveTheme(brand: Theme = {}, channel: Theme = {}): Theme {
  return { primary_color: channel.primary_color ?? brand.primary_color, accent_color: channel.accent_color ?? brand.accent_color, logo_url: channel.logo_url ?? brand.logo_url, background_color: channel.background_color ?? brand.background_color, text_color: channel.text_color ?? brand.text_color, support_url: channel.support_url ?? brand.support_url };
}
