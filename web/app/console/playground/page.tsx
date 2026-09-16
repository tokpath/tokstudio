import { headers } from "next/headers";
import { loadCatalog } from "@/lib/catalog";
import { PlaygroundClient } from "./playground-client";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { decodeModelId } from "@/lib/playground-session";
import { safeNextPath } from "@/lib/login-next";

export default async function PlaygroundPage({
  searchParams,
}: {
  searchParams: Promise<{ model?: string; from?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const { model, from } = await searchParams;
  let items = await loadCatalog(host, { limit: 100 });
  const wanted = decodeModelId(model);
  if (wanted && !items.some((item) => item.id === wanted)) {
    const extra = await loadCatalog(host, { id: wanted });
    items = [...extra, ...items];
  }

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="playground" />
      <PlaygroundClient models={items} initialModel={model} catalogHref={safeNextPath(from) || "/app/catalog"} />
    </div>
  );
}
