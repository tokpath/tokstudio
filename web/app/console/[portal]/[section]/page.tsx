import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ConsolePage } from "@/components/console-page";
import { findPortalItem, isPortalId, listConsoleSectionParams, PORTALS } from "@/lib/nav";

export function generateStaticParams() {
  return listConsoleSectionParams();
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ portal: string; section: string }>;
}): Promise<Metadata> {
  const { portal, section } = await params;
  if (!isPortalId(portal)) {
    return { title: "控制台" };
  }
  const item = findPortalItem(portal, section);
  return { title: item ? `${item.title} · ${PORTALS[portal].name}` : PORTALS[portal].name };
}

export default async function ConsoleSectionPage({
  params,
}: {
  params: Promise<{ portal: string; section: string }>;
}) {
  const { portal, section } = await params;
  if (!isPortalId(portal)) {
    notFound();
  }
  const item = findPortalItem(portal, section);
  if (!item) {
    notFound();
  }
  return <ConsolePage item={item} />;
}
