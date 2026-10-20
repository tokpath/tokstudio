import type { Theme } from "./contracts";
export function resolveTheme(brand: Theme = {}, channel: Theme = {}): Theme {
  return { primary_color: channel.primary_color ?? brand.primary_color, accent_color: channel.accent_color ?? brand.accent_color, logo_url: channel.logo_url ?? brand.logo_url, background_color: channel.background_color ?? brand.background_color, text_color: channel.text_color ?? brand.text_color, support_url: channel.support_url ?? brand.support_url };
}

/** Select readable button text for a configured brand color. */
export function brandForeground(color?: string | null): string {
  if (!color || !/^#[\da-f]{6}$/i.test(color)) return "#ffffff";
  const luminance = (hex: string) => {
    const channels = [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16) / 255).map(v => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4);
    return channels[0] * .2126 + channels[1] * .7152 + channels[2] * .0722;
  };
  const background = luminance(color), dark = luminance("#172334");
  return (1.05 / (background + .05)) >= ((background + .05) / (dark + .05)) ? "#ffffff" : "#172334";
}
