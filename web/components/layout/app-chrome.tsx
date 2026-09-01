"use client";

import { useMemo, useState } from "react";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import type { Brand } from "@/lib/brand";
import { adminNavKeys, isConsolePath, portalLinks } from "@/lib/nav";
import { CommandPalette } from "./command-palette";
import { ConsoleShell } from "./console-shell";
import { SiteFooter } from "./site-footer";
import { SiteHeader } from "./site-header";

export function AppChrome({ brand, children }: { brand?: Brand; children: React.ReactNode }) {
  const pathname = usePathname();
  const t = useTranslations("nav");
  const ta = useTranslations("admin");
  const [commandOpen, setCommandOpen] = useState(false);
  const labels = useMemo(() => {
    const next: Record<string, string> = {};
    for (const item of portalLinks) {
      next[item.key] = t(item.key);
    }
    for (const key of adminNavKeys) {
      next[key] = ta(key);
    }
    return next;
  }, [t, ta]);

  const palette = <CommandPalette open={commandOpen} onOpenChange={setCommandOpen} labels={labels} />;

  if (isConsolePath(pathname)) {
    return (
      <>
        <ConsoleShell onCommand={() => setCommandOpen(true)}>
          {children}
        </ConsoleShell>
        {palette}
      </>
    );
  }

  return (
    <>
      <SiteHeader brand={brand} onCommand={() => setCommandOpen(true)} />
      {children}
      <SiteFooter brand={brand} />
      {palette}
    </>
  );
}
