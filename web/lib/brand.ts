export type Brand = {
  id: string;
  name: string;
  logo_url?: string;
  primary_domain: string;
  api_domain: string;
  admin_domain: string;
  theme?: Record<string, string>;
};

/** OEM 只能换章的颜色。纸/碳表面不走 theme_json。 */
export function themeStyle(brand?: Brand): Record<string, string> {
  const primary = brand?.theme?.brand || brand?.theme?.primary || "#2150D6";
  const press = brand?.theme?.brand_press || brand?.theme?.brandPress || "#183CA8";
  const emphasis = brand?.theme?.brand_emphasis || brand?.theme?.brandEmphasis || primary;
  return {
    "--brand": primary,
    "--brand-primary": primary,
    "--brand-press": press,
    "--brand-emphasis": emphasis,
  };
}

export function portalForPath(pathname: string): "public" | "user" | "channel" | "partner" | "admin" | "docs" {
  if (pathname.startsWith("/admin")) return "admin";
  if (pathname.startsWith("/channel")) return "channel";
  if (pathname.startsWith("/partner")) return "partner";
  if (pathname.startsWith("/app")) return "user";
  if (pathname.startsWith("/docs")) return "docs";
  return "public";
}
