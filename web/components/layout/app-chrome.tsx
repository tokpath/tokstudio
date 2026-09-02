"use client";

import { useMemo, useState } from "react";
import { usePathname } from "next/navigation";
import type { Brand } from "@/lib/brand";
import { isAuthPath, isConsolePath } from "@/lib/nav";
import { BrandProvider } from "@/components/brand-context";
import { CommandPalette } from "./command-palette";
import { ConsoleShell } from "./console-shell";
import { SiteFooter } from "./site-footer";
import { SiteHeader } from "./site-header";
import { ViewerProvider } from "@/components/rbac/viewer-context";

export function AppChrome({ brand, children }: { brand?: Brand; children: React.ReactNode }) {
  const pathname = usePathname();
  const [commandOpen, setCommandOpen] = useState(false);
  const palette = useMemo(
    () => <CommandPalette open={commandOpen} onOpenChange={setCommandOpen} />,
    [commandOpen],
  );

  if (isAuthPath(pathname)) {
    return (
      <ViewerProvider>
        <BrandProvider brand={brand}>{children}</BrandProvider>
      </ViewerProvider>
    );
  }

  if (isConsolePath(pathname)) {
    return (
      <ViewerProvider>
        <ConsoleShell brand={brand} onCommand={() => setCommandOpen(true)}>
          {children}
        </ConsoleShell>
        {palette}
      </ViewerProvider>
    );
  }

  return (
    <ViewerProvider>
      <SiteHeader brand={brand} onCommand={() => setCommandOpen(true)} />
      {children}
      <SiteFooter brand={brand} />
      {palette}
    </ViewerProvider>
  );
}
