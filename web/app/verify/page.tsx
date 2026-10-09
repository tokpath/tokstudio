import { redirect } from "next/navigation";
import { publicRedirectHref, type PublicRedirectSearch } from "@/lib/public-redirect";

export default async function LegacyPublicPage({ searchParams }: { searchParams: Promise<PublicRedirectSearch> }) {
  redirect(publicRedirectHref("/docs", await searchParams));
}
