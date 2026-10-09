import { redirect } from "next/navigation";

export default function LegacyPublicPage() {
  redirect("/pricing");
}
