import { redirect } from "next/navigation";
import { modelEditHref } from "@/lib/catalog";
export default async function PricesPage({searchParams}:{searchParams:Promise<{model?:string}>}) {
 const {model}=await searchParams; redirect(model ? `${modelEditHref(model)}#prices` : "/admin/models");
}
