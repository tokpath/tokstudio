import { redirect } from "next/navigation";
import { walletTabHref } from "@/lib/wallet-context";
export default async function PlansPage({searchParams}:{searchParams:Promise<Record<string,string|string[]|undefined>>}){
 const search=await searchParams;const params=new URLSearchParams();
 for(const [key,value] of Object.entries(search)){if(typeof value==="string")params.set(key,value);}
 redirect(walletTabHref(params.toString(),"plans"));
}
