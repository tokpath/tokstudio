import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ConsolePage } from "@/components/console-page";
import { findPortalItem, isPortalId, listPortalParams, PORTALS } from "@/lib/nav";

export function generateStaticParams() {
  return listPortalParams();
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ portal: string }>;
}): Promise<Metadata> {
  const { portal } = await params;
  if (!isPortalId(portal)) {
    return { title: "控制台" };
  }
  const item = findPortalItem(portal);
  return { title: item ? `${item.title} · ${PORTALS[portal].name}` : PORTALS[portal].name };
}

export default async function ConsoleHomePage({
  params,
}: {
  params: Promise<{ portal: string }>;
}) {
  const { portal } = await params;
  if (!isPortalId(portal)) {
    notFound();
  }
  const item = findPortalItem(portal);
  if (!item) {
    notFound();
  }
  return <ConsolePage item={item} />;
}
