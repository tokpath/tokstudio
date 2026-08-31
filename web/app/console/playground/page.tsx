import { headers } from "next/headers";
import { loadCatalog } from "@/lib/catalog";
import { PlaygroundClient } from "./playground-client";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default async function PlaygroundPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host);
  const textModels = models.filter((m) => !m.kind || m.kind === "text").slice(0, 48);

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="playground" />
      <PlaygroundClient models={textModels} />
    </div>
  );
}
