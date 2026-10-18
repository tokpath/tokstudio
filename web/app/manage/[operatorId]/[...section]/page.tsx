import { ManagementPage } from "@/components/product/management";
import { redirect } from "next/navigation";
export default async function Page({ params, searchParams }: { params: Promise<{ operatorId: string; section: string[] }>; searchParams: Promise<{ record?: string }> }) {
  const { operatorId, section } = await params;
  const { record } = await searchParams;
  if (section[0] === "agreements") redirect(`/manage/${encodeURIComponent(operatorId)}/operators`);
  return <ManagementPage operatorID={operatorId} resource={section.map(encodeURIComponent).join("/")} record={record} />;
}
