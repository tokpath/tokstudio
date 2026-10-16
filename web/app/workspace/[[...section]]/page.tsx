import { CustomerPage } from "@/components/product/customer";
export default async function Page({ params, searchParams }: { params: Promise<{ section?: string[] }>; searchParams: Promise<{ plan?: string }> }) { const [{ section }, { plan }] = await Promise.all([params, searchParams]); return <CustomerPage section={section?.join("/") || ""} initialPlan={plan} />; }
