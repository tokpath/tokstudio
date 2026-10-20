import { expect, it } from "vitest";
import { brandForeground, resolveTheme } from "./theme";

it("keeps primary and hover actions readable for light and dark brand palettes", () => {
  for (const color of ["#ffffff", "#f8d84b", "#d1e7ff"]) expect(brandForeground(color)).toBe("#172334");
  for (const color of ["#14243d", "#2f62e8", "#552299"]) expect(brandForeground(color)).toBe("#ffffff");
});
it("uses the channel palette while retaining inherited brand presentation", () => {
  const theme = resolveTheme({ primary_color: "#2f62e8", accent_color: "#14243d", logo_url: "/brand.svg" }, { primary_color: "#f8d84b" });
  expect(theme.logo_url).toBe("/brand.svg");
  expect(brandForeground(theme.primary_color)).toBe("#172334");
  expect(brandForeground(theme.accent_color)).toBe("#ffffff");
});
