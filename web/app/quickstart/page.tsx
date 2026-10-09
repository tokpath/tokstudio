import { redirect } from "next/navigation";
import { safeNextPath } from "@/lib/login-next";

export default async function LegacyInstructions({searchParams}: {searchParams: Promise<{model?:string; next?:string}>}) {
  const search = await searchParams;
  const params = new URLSearchParams({tab:"agent"});
  const next = safeNextPath(search.next);
  if (next) params.set("next",next);
  const model = (search.model || "").trim();
  redirect(model ? `/models/${model.split("/").map(encodeURIComponent).join("/")}?${params}` : `/models${next ? `?next=${encodeURIComponent(next)}` : ""}`);
}
