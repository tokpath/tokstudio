import { ManagementPage } from "@/components/product/management";
export default async function Page({ params, searchParams }: { params: Promise<{ operatorId: string; section: string[] }>; searchParams: Promise<{ record?: string }> }) {
  const { operatorId, section } = await params;
  const { record } = await searchParams;
  return <ManagementPage operatorID={operatorId} resource={section.map(encodeURIComponent).join("/")} record={record} />;
}
