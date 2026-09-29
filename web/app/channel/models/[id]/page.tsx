import { redirect } from "next/navigation";

export default async function ChildChannelModelsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  redirect(`/channel/subchannels/${encodeURIComponent(id)}`);
}
