import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { loadCatalogPage } from "@/lib/catalog";
import { PlaygroundClient } from "./playground-client";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { decodeModelId } from "@/lib/playground-session";
import { modelEntry, useModelHref } from "@/lib/model-use";
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
  let catalogOk = page.ok;
  let catalogMessage = page.message;
  if (page.ok && wanted && !items.some((item) => item.id === wanted)) {
    const extra = await loadCatalogPage(host, { id: wanted });
    if (!extra.ok) {
      catalogOk = false;
      catalogMessage = extra.message;
    } else {
      items = [...extra.items, ...items];
    }
  }
  const found = wanted ? items.find((item) => item.id === wanted) : undefined;
  if (catalogOk && found && modelEntry(found) !== "chat") {
    redirect(useModelHref(found, from));
  }

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="playground" />
      <PlaygroundClient
        models={items}
        initialModel={model}
        catalogHref={safeNextPath(from) || "/app/catalog"}
        catalogOk={catalogOk}
        catalogMessage={catalogMessage}
      />
    </div>
  );
}
