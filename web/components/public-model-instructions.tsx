import { headers } from "next/headers";
import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { loadCatalogPage } from "@/lib/catalog";
import { PublicMain } from "@/components/public-section";
import { ModelUsagePanel } from "@/components/model-usage-panel";
export async function PublicModelInstructions({ modelID, tab = "protocol" }: {
    modelID?: string;
    tab?: "agent" | "protocol";
}) {
    const host = (await headers()).get("x-tokenhub-host") || "localhost";
    const page = await loadCatalogPage(host);
    const t = await getTranslations("keyUX");
    const model = page.items.find(item => item.id === modelID);
    return <PublicMain><h1 className="text-2xl font-semibold">{t(tab)}</h1>
  {!page.ok ? <p role="alert">{page.message || t("docsFailed")}</p> : model ? <><h2>{model.display_name}</h2><code>{model.id}</code><ModelUsagePanel model={model} models={page.items} initialTab={tab}/></> : <>
   <p>{t("chooseModel")}</p><ul className="space-y-2">{page.items.map(item => <li key={item.id}><Link className="underline" href={`/models/${item.id.split("/").map(encodeURIComponent).join("/")}?tab=${tab}`}>{item.display_name} · {item.id}</Link></li>)}</ul>
   {modelID ? <p role="alert">{t("noProtocol")} ({modelID})</p> : null}
  </>}
 </PublicMain>;
}
