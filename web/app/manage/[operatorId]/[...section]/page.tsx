import { ManagementPage } from "@/components/product/management";
import { redirect } from "next/navigation";
export default async function Page({ params, searchParams }: { params: Promise<{ operatorId: string; section: string[] }>; searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { operatorId, section } = await params;
  const search = await searchParams;
  if (section.length === 1 && section[0] === "withdrawals") {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(search)) for (const item of Array.isArray(value) ? value : value ? [value] : []) query.append(key, item);
    query.set("section", "requests");
    query.delete("view");
    redirect(`/manage/${encodeURIComponent(operatorId)}/commissions?${query}`);
  }
  if (section.length === 1 && section[0] === "procurement-invoices") redirect(`/manage/${encodeURIComponent(operatorId)}/invoices?section=received`);
  if (section[0] === "exits") redirect(`/manage/${encodeURIComponent(operatorId)}/takeovers`);
  const record = typeof search.record === "string" ? search.record : undefined;
  const supplierPool = section.length === 1 && section[0] === "providers" && typeof search.pool === "string" && (search.pool_kind === "codex" || search.pool_kind === "claude") ? { id: search.pool, kind: search.pool_kind } as const : undefined;
  return <ManagementPage operatorID={operatorId} resource={section.map(encodeURIComponent).join("/")} record={record} supplierPool={supplierPool} />;
}
