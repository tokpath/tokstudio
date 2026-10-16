import { ManagementPage } from "@/components/product/management";
export default async function Page({ params }: { params: Promise<{ operatorId: string; section: string[] }> }) { const { operatorId, section } = await params; return <ManagementPage operatorID={operatorId} resource={section.map(encodeURIComponent).join("/")} />; }
