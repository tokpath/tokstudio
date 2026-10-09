import {headers} from "next/headers";
import Link from "next/link";
import {getTranslations} from "next-intl/server";
import {loadCatalogPage} from "@/lib/catalog";
import {ModelUsagePanel} from "@/components/model-usage-panel";
import {I18nConsoleHeader} from "@/components/i18n-page-hero";
export default async function ConsoleDocsPage({searchParams}:{searchParams:Promise<{model?:string;key_id?:string;tab?:string}>}) {
 const {model,key_id,tab}=await searchParams;const host=(await headers()).get("x-tokenhub-host")||"localhost";const page=await loadCatalogPage(host);const t=await getTranslations("keyUX");const found=page.items.find(item=>item.id===model);
 return <div className="flex flex-col gap-6"><I18nConsoleHeader id="docs"/>{!page.ok?<p role="alert">{page.message||t("docsFailed")}</p>:found?<><h2 className="text-xl font-semibold">{found.display_name}</h2><code className="break-all text-sm">{found.id}</code><ModelUsagePanel model={found} models={page.items} keyID={key_id} initialTab={tab==="agent"?"agent":tab==="overview"?"overview":"protocol"}/></>:<><p>{t("chooseModel")}</p><ul className="space-y-2">{page.items.map(item=>{const params=new URLSearchParams({model:item.id,tab:tab==="agent"?"agent":"protocol"});if(key_id)params.set("key_id",key_id);return <li key={item.id}><Link className="text-brand underline" href={`/app/docs?${params}`}>{item.display_name} · {item.id}</Link></li>;})}</ul>{model?<p role="alert">{t("noProtocol")} ({model})</p>:null}</>}</div>;
}
