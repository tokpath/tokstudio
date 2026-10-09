import { redirect } from "next/navigation";
import { publicRedirectHref, type PublicRedirectSearch } from "@/lib/public-redirect";

export default async function LegacyPublicModel({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<PublicRedirectSearch>;
}) {
  const { slug } = await params;
  redirect(publicRedirectHref("/models", await searchParams, slug));
}
