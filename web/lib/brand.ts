import { apiBase } from "@/lib/api";

export type Brand = {
  id: string;
  name: string;
  logo_url?: string;
  favicon_url?: string;
  logo_dark_url?: string;
  primary_domain: string;
  api_domain: string;
  admin_domain: string;
  theme?: Record<string, string>;
};

export const LOGO_MAX_BYTES = 128 * 1024;
export const FAVICON_MAX_BYTES = 64 * 1024;
export const OG_MAX_BYTES = 512 * 1024;
export const LOGO_MIN_SIDE = 64;
export const LOGO_MAX_SIDE = 1024;
export const LOGO_MAX_RATIO = 4;
export const LOGO_DISPLAY_CLASS = "h-6 max-w-24 w-auto rounded-[6px] object-contain";

/** OEM 只能换章的颜色。纸/碳表面不走 theme_json。 */
export function themeStyle(brand?: Brand): Record<string, string> {
  const theme = brand?.theme || {};
  const primary = theme.brand || theme.primary || "#2150D6";
  const press = theme.brand_press || theme.brandPress || "#183CA8";
  const soft = theme.brand_soft || "#DCE6FB";
  const softDark = theme.brand_soft_dark || "#243056";
  const emphasis = theme.brand_emphasis || theme.brandEmphasis || primary;
  const emphasisDark = theme.brand_emphasis_dark || "#8AA4FF";
  const onBrand = theme.on_brand || "#FFFFFF";
  return {
    "--oem-brand": primary,
    "--oem-brand-press": press,
    "--oem-brand-soft": soft,
    "--oem-brand-soft-dark": softDark,
    "--oem-brand-emphasis": emphasis,
    "--oem-brand-emphasis-dark": emphasisDark,
    "--oem-on-brand": onBrand,
  };
}

/** 本站资源走 /api 前缀；外链原样返回。 */
export function publicAssetURL(path?: string): string | undefined {
  if (!path) return undefined;
  if (path.startsWith("http://") || path.startsWith("https://")) return path;
  if (path.startsWith("/api/")) return path;
  return `${apiBase}${path.startsWith("/") ? path : `/${path}`}`;
}

export function logoSrc(brand?: Brand, dark?: boolean): string | undefined {
  if (dark && brand?.logo_dark_url) return publicAssetURL(brand.logo_dark_url);
  return publicAssetURL(brand?.logo_url);
}

export type AssetKind = "logo" | "logo_dark" | "favicon" | "og_image";

export function assetByteLimit(kind: AssetKind): number {
  if (kind === "favicon") return FAVICON_MAX_BYTES;
  if (kind === "og_image") return OG_MAX_BYTES;
  return LOGO_MAX_BYTES;
}

export function describeAssetLimit(kind: AssetKind): string {
  if (kind === "favicon") return "Favicon：PNG/ICO，≤64KiB，恰好 32×32 或 48×48";
  if (kind === "og_image") return "分享图：PNG/JPEG/WebP，≤512KiB，恰好 1200×630";
  return "Logo：PNG/WebP/SVG，≤128KiB，短边 64–1024px，宽高比 1:1～4:1，页上高 24px";
}

export async function inspectLocalAsset(kind: AssetKind, file: File): Promise<string | null> {
  if (file.size > assetByteLimit(kind)) {
    return `文件 ${file.size} 字节，超过 ${assetByteLimit(kind)} 字节上限`;
  }
  if (kind !== "logo" && kind !== "logo_dark" && file.type === "image/svg+xml") {
    return "只有 Logo 可以上传 SVG";
  }
  if ((kind === "logo" || kind === "logo_dark") && file.type === "image/jpeg") {
    return "Logo 不能用 JPEG";
  }
  if (file.type === "image/svg+xml") return null;
  const bitmap = await createImageBitmap(file).catch(() => null);
  if (!bitmap) return "无法读取图片尺寸";
  const { width, height } = bitmap;
  bitmap.close();
  if (kind === "favicon" && !((width === 32 && height === 32) || (width === 48 && height === 48))) {
    return `Favicon 现在是 ${width}×${height}，必须是 32×32 或 48×48`;
  }
  if (kind === "og_image" && (width !== 1200 || height !== 630)) {
    return `分享图现在是 ${width}×${height}，必须是 1200×630`;
  }
  if (kind === "logo" || kind === "logo_dark") {
    const short = Math.min(width, height);
    const long = Math.max(width, height);
    const ratio = width / height;
    if (short < LOGO_MIN_SIDE || long > LOGO_MAX_SIDE || ratio < 1 || ratio > LOGO_MAX_RATIO) {
      return `Logo 现在是 ${width}×${height}，短边需 64–1024px，宽高比 1:1～4:1`;
    }
  }
  return null;
}
