import { headers } from "next/headers";
import ExamplesPanel from "../examples-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { loadCatalogPage } from "@/lib/catalog";
import { catalogModelUsable, exampleCurl, examplePath, modelEntry, useModelHref } from "@/lib/model-use";
import { decodeModelId } from "@/lib/playground-session";
import { redirect } from "next/navigation";
import { safeNextPath } from "@/lib/login-next";

export default async function ConsoleDocsPage({
  searchParams,
}: {
  searchParams: Promise<{ model?: string; from?: string }>;
}) {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const { model, from } = await searchParams;
  const wanted = decodeModelId(model);
  let catalogOk = true;
  let catalogMessage: string | undefined;
  let modelError: "missing" | "unavailable" | undefined;
  let focusCurl: string | undefined;
  let focusPath: string | undefined;
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
      } else if (modelEntry(found) !== "docs") {
        redirect(useModelHref(found, from));
      } else {
        focusPath = examplePath(found);
        focusCurl = exampleCurl(found.id, focusPath, host);
      }
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="docs" />
      <ExamplesPanel
        catalogHref={safeNextPath(from) || "/app/catalog"}
        catalogOk={catalogOk}
        catalogMessage={catalogMessage}
        requestedModel={wanted || undefined}
        modelError={modelError}
        focusCurl={focusCurl}
        focusPath={focusPath}
      />
    </div>
  );
}
