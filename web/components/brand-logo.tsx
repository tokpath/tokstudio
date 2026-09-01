"use client";

import { useTheme } from "next-themes";
import type { Brand } from "@/lib/brand";
import { LOGO_DISPLAY_CLASS, logoSrc } from "@/lib/brand";
import { BrandMark } from "@/components/brand-mark";

export function BrandLogo({ brand, className = LOGO_DISPLAY_CLASS }: { brand?: Brand; className?: string }) {
  const { resolvedTheme } = useTheme();
  const src = logoSrc(brand, resolvedTheme === "dark");
  if (!src) {
    return <BrandMark />;
  }
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={src} alt="" className={className} />
  );
}
