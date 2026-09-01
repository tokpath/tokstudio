"use client";

import { useMemo, useState } from "react";
import { usePathname } from "next/navigation";
import type { Brand } from "@/lib/brand";
import { isConsolePath } from "@/lib/nav";
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
