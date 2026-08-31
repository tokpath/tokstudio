"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  adminGroups,
  channelSections,
  consoleItemHref,
  partnerSections,
  portalLinks,
  userSections,
  userSettingsNav,
  PUBLIC_NAV,
  PUBLIC_NAV_MORE,
} from "@/lib/nav";
import { PUBLIC_PAGE_SPECS } from "@/lib/public-site";

type Item = { href: string; label: string; group: string };

const PUBLIC_TITLE: Record<string, string> = {
  "/": "home",
  "/models": "models",
  "/quickstart": "quickstart",
  "/docs": "docs",
  "/docs/integrations": "docsIntegrations",
  "/docs/develop": "docsDevelop",
  "/docs/changelog": "docsChangelog",
  "/enterprise": "enterprise",
  "/trust": "trust",
  "/trust/subprocessors": "trustSubprocessors",
  "/best-value": "bestValue",
  "/model-finder": "modelFinder",
  "/vibe-coding": "vibeCoding",
  "/video": "video",
  "/image": "image",
  "/leaderboards/models": "leaderboardsModels",
  "/leaderboards/apps": "leaderboardsApps",
  "/leaderboards/labs": "leaderboardsLabs",
  "/vs/openrouter": "vsOpenrouter",
  "/compare": "compare",
  "/promo": "promo",
  "/promo/august": "promoAugust",
  "/desktop": "desktop",
  "/verify": "verify",
  "/awesome-ofox": "awesome",
  "/pricing": "pricing",
  "/blog": "blog",
  "/terms": "terms",
  "/privacy": "privacy",
};

const CONSOLE_TITLE: Record<string, string> = {
  "/app": "overview",
  "/app/playground": "playground",
  "/app/keys": "keys",
  "/app/catalog": "catalog",
  "/app/usage": "usage",
  "/app/activity": "activity",
  "/app/wallet": "wallet",
  "/app/plans": "plans",
  "/app/media": "media",
  "/app/referral": "referral",
  "/app/docs": "docs",
  "/app/settings": "settings",
  "/app/settings/team": "team",
  "/app/settings/members": "members",
  "/app/settings/billing": "billing",
  "/app/settings/quotas": "quotas",
  "/app/settings/apps": "apps",
  "/app/settings/webhooks": "webhooks",
};

export function CommandPalette({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const router = useRouter();
  const [q, setQ] = useState("");
  const tNav = useTranslations("nav");
  const tChrome = useTranslations("chrome");
  const tMega = useTranslations("mega");
  const tUser = useTranslations("userNav");
  const tChannel = useTranslations("channelNav");
  const tPartner = useTranslations("partnerNav");
  const tAdmin = useTranslations("admin");
  const tPublic = useTranslations("public");
  const tHome = useTranslations("home");
  const tLogin = useTranslations("login");
  const tOverview = useTranslations("overview");
  const tConsole = useTranslations("console");

  const items = useMemo<Item[]>(() => {
    const portals = portalLinks.map((item) => ({
      href: item.href,
      label: tNav(item.key),
      group: tChrome("groupPortal"),
    }));
    const publicPages = PUBLIC_PAGE_SPECS.filter((p) => !p.auth).map((p) => {
      let label = p.label;
      if (p.href === "/") {
        label = tHome("pageTitle");
      } else if (p.href === "/login") {
        label = tLogin("title");
      } else if (PUBLIC_TITLE[p.href]) {
        label = tPublic(`${PUBLIC_TITLE[p.href]}.title`);
      }
      return { href: p.href, label, group: tChrome("groupPublic") };
    });
    const navExtra = [...PUBLIC_NAV, ...PUBLIC_NAV_MORE].map((item) => ({
      href: item.href,
      label: tMega(item.key),
      group: tChrome("groupPublic"),
    }));
    const user = [...userSections, ...userSettingsNav].map((item) => ({
      href: consoleItemHref(item, "/app"),
      label: tUser(item.key),
      group: tChrome("groupUser"),
    }));
    const channel = channelSections.map((item) => ({
      href: consoleItemHref(item, "/channel"),
      label: tChannel(item.key),
      group: tChrome("groupChannel"),
    }));
    const partner = partnerSections.map((item) => ({
      href: consoleItemHref(item, "/partner"),
      label: tPartner(item.key),
      group: tChrome("groupPartner"),
    }));
    const admin = adminGroups.flatMap((group) =>
      group.items.map((item) => ({ href: item.href, label: tAdmin(item.key), group: tChrome("groupAdmin") })),
    );
    const authPages = PUBLIC_PAGE_SPECS.filter((p) => p.auth).map((p) => {
      const consoleKey = CONSOLE_TITLE[p.href];
      const label = consoleKey
        ? p.href === "/app"
          ? tOverview("title")
          : tConsole(`${consoleKey}.title`)
        : p.label;
      return { href: p.href, label, group: tChrome("groupUser") };
    });
    const merged = [...portals, ...publicPages, ...navExtra, ...user, ...channel, ...partner, ...admin, ...authPages];
    const seen = new Set<string>();
    return merged.filter((item) => {
      const key = `${item.group}:${item.href}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }, [tNav, tChrome, tMega, tUser, tChannel, tPartner, tAdmin, tPublic, tHome, tLogin, tOverview, tConsole]);

  const filtered = items.filter((item) => {
    const hay = `${item.label} ${item.href} ${item.group}`.toLowerCase();
    return hay.includes(q.trim().toLowerCase());
  });

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        onOpenChange(!open);
      }
      if (event.key === "Escape") {
        onOpenChange(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onOpenChange, open]);

  useEffect(() => {
    if (!open) {
      setQ("");
    }
  }, [open]);

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-[80] flex items-start justify-center bg-scrim px-4 pt-[12vh]" onClick={() => onOpenChange(false)}>
      <div
        className="w-full max-w-xl overflow-hidden rounded-card border border-hairline bg-canvas-raised"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-label={tChrome("palette")}
      >
        <input
          autoFocus
          value={q}
          onChange={(event) => setQ(event.target.value)}
          placeholder={tChrome("palettePlaceholder")}
          className="h-12 w-full border-b border-hairline bg-transparent px-4 text-sm text-ink placeholder:text-ink-mute"
        />
        <ul className="th-scrollbar max-h-80 overflow-auto p-2">
          {filtered.slice(0, 16).map((item) => (
            <li key={`${item.group}-${item.href}`}>
              <button
                className="flex w-full items-center justify-between rounded-control px-3 py-2 text-left text-sm text-ink hover:bg-brand-soft"
                onClick={() => {
                  onOpenChange(false);
                  router.push(item.href);
                }}
              >
                <span>{item.label}</span>
                <span className="text-xs text-ink-mute">{item.group}</span>
              </button>
            </li>
          ))}
          {filtered.length === 0 ? <li className="px-3 py-6 text-center text-sm text-ink-mute">{tChrome("paletteEmpty")}</li> : null}
        </ul>
        <p className="border-t border-hairline px-4 py-2 text-xs text-ink-mute">{tChrome("paletteHint")}</p>
      </div>
    </div>
  );
}
