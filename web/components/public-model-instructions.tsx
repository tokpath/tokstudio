import { headers } from "next/headers";
import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { loadCatalogPage } from "@/lib/catalog";
import { PublicMain } from "@/components/public-section";
import { ModelUsagePanel } from "@/components/model-usage-panel";
import {instructionsHref, instructionsQuery, type InstructionsQuery} from "@/lib/model-instructions-context";
export async function PublicModelInstructions({ modelID, tab = "protocol", query = {}, pathname = "/docs" }: {
    modelID?: string;
    tab?: "agent" | "protocol";
    query?: InstructionsQuery;
    pathname?: string;
}) {
    const host = (await headers()).get("x-tokenhub-host") || "localhost";
    const page = await loadCatalogPage(host, modelID ? {id: modelID} : {});
    const t = await getTranslations("keyUX");
    const model = page.items.find(item => item.id === modelID);
    return <PublicMain className="gap-6 sm:gap-8"><h1 className="text-2xl font-semibold">{t("instructions")}</h1>
  {!page.ok ? <p role="alert">{page.message || t("docsFailed")}</p> : model ? <><div className="space-y-2"><h2>{model.display_name}</h2><code>{model.id}</code></div><ModelUsagePanel model={model} models={page.items} keyID={query.key_id} initialTab={tab} initialHref={instructionsHref(pathname, query, model.id, tab)}/></> : <>
   <p>{t("chooseModel")}</p><ul className="space-y-2">{page.items.map(item => <li key={item.id}><Link className="underline" href={`/docs?${instructionsQuery(query, item.id, tab)}`}>{item.display_name} · {item.id}</Link></li>)}</ul>
   {modelID ? <p role="alert">{t("noProtocol")} ({modelID})</p> : null}
  </>}
 </PublicMain>;
}
