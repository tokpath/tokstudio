import { ManagementPage } from "@/components/product/management";
import { redirect } from "next/navigation";
export default async function Page({ params, searchParams }: { params: Promise<{ operatorId: string; section: string[] }>; searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { operatorId, section } = await params;
  const search = await searchParams;
  if (section.length === 1 && section[0] === "withdrawals") {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(search)) for (const item of Array.isArray(value) ? value : value ? [value] : []) query.append(key, item);
    query.set("section", search.view === "methods" ? "methods" : "requests");
    query.delete("view");
    redirect(`/manage/${encodeURIComponent(operatorId)}/commissions?${query}`);
  }
  const record = typeof search.record === "string" ? search.record : undefined;
  return <ManagementPage operatorID={operatorId} resource={section.map(encodeURIComponent).join("/")} record={record} />;
}
