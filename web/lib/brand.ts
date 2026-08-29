export type Brand = {
  id: string;
  name: string;
  logo_url?: string;
  primary_domain: string;
  api_domain: string;
  admin_domain: string;
  theme?: Record<string, string>;
};

export function themeStyle(brand?: Brand): Record<string, string> {
  return {
    "--brand-primary": brand?.theme?.primary || "#22d3ee",
    "--brand-background": brand?.theme?.background || "#020617",
  };
}

export function portalForPath(pathname: string): "public" | "user" | "channel" | "admin" | "docs" {
  if (pathname.startsWith("/admin")) return "admin";
  if (pathname.startsWith("/channel")) return "channel";
  if (pathname.startsWith("/app")) return "user";
  if (pathname.startsWith("/docs")) return "docs";
  return "public";
}
