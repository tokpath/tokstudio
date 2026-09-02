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

export function AppChrome({ brand, children }: { brand?: Brand; children: React.ReactNode }) {
  const pathname = usePathname();
  const [commandOpen, setCommandOpen] = useState(false);
  const palette = useMemo(
    () => <CommandPalette open={commandOpen} onOpenChange={setCommandOpen} />,
    [commandOpen],
  );

  if (isAuthPath(pathname)) {
    return <BrandProvider brand={brand}>{children}</BrandProvider>;
  }

  if (isConsolePath(pathname)) {
    return (
      <>
        <ConsoleShell brand={brand} onCommand={() => setCommandOpen(true)}>
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
