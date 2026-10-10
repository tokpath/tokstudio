import { redirect } from "next/navigation";
import { modelEditHref } from "@/lib/catalog";
import { appendReturnContext, safeReturnHref } from "@/lib/return-context";
export default async function PricesPage({ searchParams }: { searchParams: Promise<{model?:string;return_to?:string}> }) {
 const {model,return_to}=await searchParams;
 const target=model ? `${modelEditHref(model)}#prices` : "/admin/models";
 redirect(return_to ? appendReturnContext(target,safeReturnHref(return_to,"/admin/models")) : target);
}
