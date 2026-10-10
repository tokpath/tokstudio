import { redirect } from "next/navigation";
export default async function MergedPage({searchParams}:{searchParams:Promise<Record<string,string|string[]|undefined>>}) {
 const params=new URLSearchParams();const query=await searchParams;for(const [key,value] of Object.entries(query))if(typeof value==="string")params.set(key,value);params.set("tab","requests");params.set("state","pending_reconciliation");redirect("/app/usage"+(params.size?`?${params}`:""));
}
