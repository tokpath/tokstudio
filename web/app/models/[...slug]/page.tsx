import {headers} from "next/headers";
import {notFound} from "next/navigation";
import Link from "next/link";
import {loadCatalogPage} from "@/lib/catalog";
import {ModelUsagePanel} from "@/components/model-usage-panel";
import {PublicMain} from "@/components/public-section";
export default async function ModelDetailPage({params,searchParams}:{params:Promise<{slug:string[]}>;searchParams:Promise<{key_id?:string;tab?:string}>}) {
 const {slug}=await params;const {key_id,tab}=await searchParams;const host=(await headers()).get("x-tokenhub-host")||"localhost";const page=await loadCatalogPage(host,{id:slug.join("/")});const model=page.items[0];if(!page.ok)return <PublicMain><p role="alert">{page.message}</p></PublicMain>;if(!model)notFound();
 return <PublicMain><Link href="/models" className="text-brand underline">Models</Link><h1 className="th-display-sm">{model.display_name}</h1><code className="break-all text-sm">{model.id}</code><ModelUsagePanel model={model} keyID={key_id} initialTab={tab==="agent"?"agent":tab==="protocol"?"protocol":"overview"}/></PublicMain>;
}
