import { headers } from "next/headers";
import { redirect } from "next/navigation";
import MediaPanel from "../media-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { loadCatalogPage } from "@/lib/catalog";
import { catalogModelUsable, modelEntry, useModelHref } from "@/lib/model-use";
import { decodeModelId } from "@/lib/playground-session";
import { safeNextPath } from "@/lib/login-next";

export default async function MediaPage({
  searchParams,
}: {
  searchParams: Promise<{ kind?: string; model?: string; from?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const { kind, model, from } = await searchParams;
  const wanted = decodeModelId(model);
  let catalogOk = true;
  let catalogMessage: string | undefined;
  let modelError: "missing" | "unavailable" | undefined;
  let entryKind = kind === "video" || kind === "image" ? kind : undefined;
  if (wanted) {
    const page = await loadCatalogPage(host, { id: wanted });
    if (!page.ok) {
      catalogOk = false;
      catalogMessage = page.message;
    } else {
      const found = page.items.find((item) => item.id === wanted);
      if (!found) {
        modelError = "missing";
      } else if (!catalogModelUsable(found)) {
        modelError = "unavailable";
      } else {
        const entry = modelEntry(found);
        if (entry !== "image" && entry !== "video") {
          redirect(useModelHref(found, from));
        }
        entryKind = entry;
      }
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="media" />
      <MediaPanel
        initialKind={entryKind}
        initialModel={wanted || undefined}
        catalogHref={safeNextPath(from) || "/app/catalog"}
        catalogOk={catalogOk}
        catalogMessage={catalogMessage}
        modelError={modelError}
      />
    </div>
  );
}
