import { describe, expect, it } from "vitest";
import { Circle } from "lucide-react";
import {
  adminGroups,
  channelSections,
  partnerSections,
  userSections,
  userSettingsNav,
} from "./nav";
import { MEGA_MENUS, TOP_LINKS } from "./mega-nav";
import { FOOTER_GROUPS, PUBLIC_PAGE_SPECS } from "./public-site";
import { iconForHref, iconForKind, iconForMegaLink, iconForPublicPage, KIND_ICONS, PUBLIC_PAGE_ICONS } from "./page-icons";

describe("page icons", () => {
  it("covers every console, channel, partner, admin, and settings href", () => {
    const hrefs = [
      ...userSections.map((item) => item.href),
      ...userSettingsNav.map((item) => item.href),
      ...channelSections.map((item) => item.href),
      ...partnerSections.map((item) => item.href),
      ...adminGroups.flatMap((group) => group.items.map((item) => item.href)),
    ];
    for (const href of hrefs) {
      expect(iconForHref(href), href).not.toBe(Circle);
    }
  });

  it("covers public page specs, footer links, and mega menu entries", () => {
    for (const page of PUBLIC_PAGE_SPECS) {
      expect(iconForHref(page.href), page.href).not.toBe(Circle);
    }
    for (const group of FOOTER_GROUPS) {
      for (const link of group.links) {
        expect(iconForHref(link.href), link.href).not.toBe(Circle);
      }
    }
    for (const menu of MEGA_MENUS) {
      for (const col of menu.columns) {
        for (const link of col.links) {
          expect(iconForMegaLink(link), `${link.href}:${link.labelKey || link.literal}`).not.toBe(Circle);
        }
      }
    }
    for (const link of TOP_LINKS) {
      expect(iconForHref(link.href), link.href).not.toBe(Circle);
    }
  });

  it("maps public hero ids and model kinds", () => {
    for (const id of Object.keys(PUBLIC_PAGE_ICONS)) {
      expect(iconForPublicPage(id)).not.toBe(Circle);
    }
    for (const kind of Object.keys(KIND_ICONS)) {
      expect(iconForKind(kind)).toBe(KIND_ICONS[kind]);
    }
    expect(iconForHref("/models/openai/gpt-5.6-sol")).not.toBe(Circle);
    expect(iconForHref("/enter")).not.toBe(Circle);
  });
});
