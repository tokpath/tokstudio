import { headers } from "next/headers";
import { loadCatalogPage } from "@/lib/catalog";
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
  const wanted = decodeModelId(model);
  const page = await loadCatalogPage(host, { limit: 100 });
  let items = page.items;
  if (page.ok && wanted && !items.some((item) => item.id === wanted)) {
    const extra = await loadCatalogPage(host, { id: wanted });
    if (extra.ok) {
      items = [...extra.items, ...items];
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="playground" />
      <PlaygroundClient
        models={items}
        initialModel={model}
        catalogHref={safeNextPath(from) || "/app/catalog"}
        catalogOk={page.ok}
        catalogMessage={page.message}
      />
    </div>
  );
}
