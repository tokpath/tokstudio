import { notFound } from "next/navigation";
import type { ReactNode } from "react";
import { ConsoleShell } from "@/components/console-shell";
import { isPortalId } from "@/lib/nav";

export default async function ConsoleLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ portal: string }>;
}) {
  const { portal } = await params;
  if (!isPortalId(portal)) {
    notFound();
  }
  return <ConsoleShell portal={portal}>{children}</ConsoleShell>;
}
