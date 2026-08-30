import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ConsolePage } from "@/components/console-page";
import { findPortalItem, isPortalId, listConsoleParams, PORTALS } from "@/lib/nav";

export function generateStaticParams() {
  return listConsoleParams();
}

export const dynamicParams = false;

export async function generateMetadata({
  params,
}: {
  params: Promise<{ portal: string; slug?: string[] }>;
}): Promise<Metadata> {
  const { portal, slug } = await params;
  if (!isPortalId(portal)) {
    return { title: "控制台" };
  }
  const item = findPortalItem(portal, slug);
  return { title: item ? `${item.title} · ${PORTALS[portal].name}` : PORTALS[portal].name };
}

export default async function ConsoleRoutePage({
  params,
}: {
  params: Promise<{ portal: string; slug?: string[] }>;
}) {
  const { portal, slug } = await params;
  if (!isPortalId(portal)) {
    notFound();
  }
  const item = findPortalItem(portal, slug);
  if (!item) {
    notFound();
  }
  return <ConsolePage item={item} />;
}
