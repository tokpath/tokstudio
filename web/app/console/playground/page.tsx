import { headers } from "next/headers";
import { loadCatalog } from "@/lib/catalog";
import { PlaygroundClient } from "./playground-client";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default async function PlaygroundPage({
  searchParams,
}: {
  searchParams: Promise<{ model?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const { model } = await searchParams;
  const textModels = await loadCatalog(host, { kind: "text", limit: 48 });

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="playground" />
      <PlaygroundClient models={textModels} initialModel={model} />
    </div>
  );
}
